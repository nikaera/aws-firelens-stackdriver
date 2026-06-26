data "aws_availability_zones" "available" {
  state = "available"
}

data "aws_caller_identity" "current" {}

data "google_project" "current" {
  project_id = var.gcp_project_id
}

locals {
  name                 = var.project_name
  gcp_region           = "asia-northeast1"
  cloud_logging_log_id = local.name
  wif_pool_id          = substr("${local.name}-pool", 0, 32)
  wif_provider_id      = substr("${local.name}-aws", 0, 32)
  availability_zones   = slice(data.aws_availability_zones.available.names, 0, 2)

  common_tags = merge(
    {
      Project   = local.name
      ManagedBy = "Terraform"
    },
    var.tags,
  )

  app_image      = "${aws_ecr_repository.app.repository_url}:${var.app_image_tag}"
  firelens_image = "${aws_ecr_repository.fluentbit.repository_url}:${var.firelens_image_tag}"
  public_subnets = tomap({
    for index, az in local.availability_zones : az => {
      az         = az
      cidr_block = cidrsubnet(var.vpc_cidr, 8, index)
    }
  })
}

resource "aws_vpc" "this" {
  cidr_block           = var.vpc_cidr
  enable_dns_hostnames = true
  enable_dns_support   = true

  tags = merge(local.common_tags, {
    Name = "${local.name}-vpc"
  })
}

resource "aws_internet_gateway" "this" {
  vpc_id = aws_vpc.this.id

  tags = merge(local.common_tags, {
    Name = "${local.name}-igw"
  })
}

resource "aws_subnet" "public" {
  for_each = local.public_subnets

  vpc_id                  = aws_vpc.this.id
  availability_zone       = each.value.az
  cidr_block              = each.value.cidr_block
  map_public_ip_on_launch = true

  tags = merge(local.common_tags, {
    Name = "${local.name}-${each.value.az}"
  })
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.this.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.this.id
  }

  tags = merge(local.common_tags, {
    Name = "${local.name}-public"
  })
}

resource "aws_route_table_association" "public" {
  for_each = local.public_subnets

  subnet_id      = aws_subnet.public[each.key].id
  route_table_id = aws_route_table.public.id
}

resource "aws_security_group" "service" {
  name        = "${local.name}-service"
  description = "Ingress for the demo app"
  vpc_id      = aws_vpc.this.id

  dynamic "ingress" {
    for_each = var.allowed_ingress_cidrs

    content {
      description = "HTTP"
      from_port   = var.container_port
      to_port     = var.container_port
      protocol    = "tcp"
      cidr_blocks = [ingress.value]
    }
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = merge(local.common_tags, {
    Name = "${local.name}-sg"
  })
}

resource "aws_cloudwatch_log_group" "firelens" {
  name              = "/ecs/${local.name}/firelens"
  retention_in_days = 7

  tags = local.common_tags
}

resource "aws_ecr_repository" "app" {
  name                 = "${local.name}-app"
  image_tag_mutability = var.ecr_image_tag_mutability
  force_delete         = var.ecr_force_delete

  image_scanning_configuration {
    scan_on_push = true
  }

  tags = local.common_tags
}

resource "aws_ecr_repository" "fluentbit" {
  name                 = "${local.name}-fluentbit"
  image_tag_mutability = var.ecr_image_tag_mutability
  force_delete         = var.ecr_force_delete

  image_scanning_configuration {
    scan_on_push = true
  }

  tags = local.common_tags
}

data "aws_iam_policy_document" "ecs_task_assume_role" {
  statement {
    effect = "Allow"

    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }

    actions = ["sts:AssumeRole"]
  }
}

resource "aws_iam_role" "execution" {
  name               = "${local.name}-execution-role"
  assume_role_policy = data.aws_iam_policy_document.ecs_task_assume_role.json

  tags = local.common_tags
}

resource "aws_iam_role_policy_attachment" "execution" {
  role       = aws_iam_role.execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

resource "aws_iam_role" "task" {
  name               = "${local.name}-task-role"
  assume_role_policy = data.aws_iam_policy_document.ecs_task_assume_role.json

  tags = local.common_tags
}

resource "aws_ecs_cluster" "this" {
  name = local.name

  tags = local.common_tags
}

resource "google_project_service" "required" {
  for_each = toset([
    "iam.googleapis.com",
    "iamcredentials.googleapis.com",
    "logging.googleapis.com",
    "sts.googleapis.com",
  ])

  project            = var.gcp_project_id
  service            = each.value
  disable_on_destroy = false
}

resource "google_service_account" "firelens" {
  account_id   = substr(replace(local.name, "-", ""), 0, 24)
  display_name = "FireLens Stackdriver demo"
  description  = "Impersonated by ECS FireLens through AWS Workload Identity Federation."

  depends_on = [google_project_service.required]
}

resource "google_project_iam_member" "logging_writer" {
  project = var.gcp_project_id
  role    = "roles/logging.logWriter"
  member  = "serviceAccount:${google_service_account.firelens.email}"
}

resource "google_iam_workload_identity_pool" "aws" {
  workload_identity_pool_id = local.wif_pool_id
  display_name              = "AWS FireLens"
  description               = "Allows ECS FireLens task role credentials to impersonate the logging service account."

  depends_on = [google_project_service.required]
}

resource "google_iam_workload_identity_pool_provider" "aws" {
  workload_identity_pool_id          = google_iam_workload_identity_pool.aws.workload_identity_pool_id
  workload_identity_pool_provider_id = local.wif_provider_id
  display_name                       = "AWS account ${data.aws_caller_identity.current.account_id}"
  description                        = "Trusts AWS STS GetCallerIdentity tokens from the demo AWS account."

  aws {
    account_id = data.aws_caller_identity.current.account_id
  }

  attribute_mapping = {
    "google.subject"     = "assertion.arn"
    "attribute.account"  = "assertion.account"
    "attribute.aws_role" = "assertion.arn.extract('assumed-role/{role_name}/')"
  }

  attribute_condition = "assertion.arn.startsWith('arn:aws:sts::${data.aws_caller_identity.current.account_id}:assumed-role/${aws_iam_role.task.name}/')"
}

resource "google_service_account_iam_member" "wif_user" {
  service_account_id = google_service_account.firelens.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "principalSet://iam.googleapis.com/projects/${data.google_project.current.number}/locations/global/workloadIdentityPools/${google_iam_workload_identity_pool.aws.workload_identity_pool_id}/attribute.aws_role/${aws_iam_role.task.name}"
}

resource "aws_ecs_task_definition" "this" {
  family                   = local.name
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = tostring(var.task_cpu)
  memory                   = tostring(var.task_memory)
  execution_role_arn       = aws_iam_role.execution.arn
  task_role_arn            = aws_iam_role.task.arn

  runtime_platform {
    operating_system_family = "LINUX"
    cpu_architecture        = "ARM64"
  }

  container_definitions = jsonencode([
    {
      name      = "log_router"
      image     = local.firelens_image
      essential = true
      firelensConfiguration = {
        type = "fluentbit"
        options = {
          "enable-ecs-log-metadata" = "false"
          "config-file-type"        = "file"
          "config-file-value"       = "/fluent-bit/etc/pipeline.conf"
        }
      }
      environment = [
        {
          name  = "AWS_REGION"
          value = var.aws_region
        },
        {
          name  = "GCP_PROJECT_ID"
          value = var.gcp_project_id
        },
        {
          name  = "GCP_PROJECT_NUMBER"
          value = data.google_project.current.number
        },
        {
          name  = "CLOUD_LOGGING_LOG_ID"
          value = local.cloud_logging_log_id
        },
        {
          name  = "WIF_POOL_ID"
          value = google_iam_workload_identity_pool.aws.workload_identity_pool_id
        },
        {
          name  = "WIF_PROVIDER_ID"
          value = google_iam_workload_identity_pool_provider.aws.workload_identity_pool_provider_id
        },
        {
          name  = "GOOGLE_SERVICE_ACCOUNT"
          value = google_service_account.firelens.email
        },
      ]
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          "awslogs-group"         = aws_cloudwatch_log_group.firelens.name
          "awslogs-region"        = var.aws_region
          "awslogs-stream-prefix" = "firelens"
        }
      }
      mountPoints            = []
      portMappings           = []
      readonlyRootFilesystem = var.readonly_root_filesystem
      systemControls         = []
      user                   = var.firelens_container_user
      volumesFrom            = []
    },
    {
      name      = "app"
      image     = local.app_image
      essential = true
      portMappings = [
        {
          containerPort = var.container_port
          hostPort      = var.container_port
          protocol      = "tcp"
        }
      ]
      dependsOn = [
        {
          containerName = "log_router"
          condition     = "START"
        }
      ]
      environment = [
        {
          name  = "PORT"
          value = tostring(var.container_port)
        },
        {
          name  = "SERVICE_NAME"
          value = local.name
        },
      ]
      logConfiguration = {
        logDriver = "awsfirelens"
      }
      mountPoints            = []
      readonlyRootFilesystem = var.readonly_root_filesystem
      systemControls         = []
      volumesFrom            = []
    },
  ])

  tags = local.common_tags

  depends_on = [
    aws_iam_role_policy_attachment.execution,
    google_project_iam_member.logging_writer,
    google_service_account_iam_member.wif_user,
  ]
}

resource "aws_ecs_service" "this" {
  name            = local.name
  cluster         = aws_ecs_cluster.this.id
  task_definition = aws_ecs_task_definition.this.arn
  desired_count   = var.desired_count
  launch_type     = "FARGATE"

  network_configuration {
    assign_public_ip = true
    subnets          = [for subnet in aws_subnet.public : subnet.id]
    security_groups  = [aws_security_group.service.id]
  }

  tags = local.common_tags
}
