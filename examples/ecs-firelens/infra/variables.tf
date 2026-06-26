variable "project_name" {
  description = "Prefix used for resource names."
  type        = string
  default     = "firelens-stackdriver-demo"
}

variable "aws_region" {
  description = "AWS region for ECS and networking resources."
  type        = string
  default     = "ap-northeast-1"
}

variable "aws_profile" {
  description = "Optional AWS shared config profile name. Leave null to use the default AWS credential chain."
  type        = string
  default     = null
}

variable "gcp_project_id" {
  description = "Google Cloud project ID that hosts Cloud Logging and workload identity resources."
  type        = string

  validation {
    condition     = length(trimspace(var.gcp_project_id)) > 0
    error_message = "gcp_project_id is required."
  }
}

variable "vpc_cidr" {
  description = "CIDR block for the demo VPC."
  type        = string
  default     = "10.42.0.0/16"
}

variable "container_port" {
  description = "Port exposed by the Go app container."
  type        = number
  default     = 8080
}

variable "desired_count" {
  description = "Desired ECS service task count."
  type        = number
  default     = 1
}

variable "task_cpu" {
  description = "Fargate task CPU units."
  type        = number
  default     = 256
}

variable "task_memory" {
  description = "Fargate task memory in MiB."
  type        = number
  default     = 512
}

variable "app_image_tag" {
  description = "Image tag consumed by the sample app container."
  type        = string
  default     = "latest"
}

variable "firelens_image_tag" {
  description = "Image tag consumed by the sample FireLens container."
  type        = string
  default     = "latest"
}

variable "ecr_force_delete" {
  description = "Delete ECR repositories even when they contain images. Keep false outside disposable test environments."
  type        = bool
  default     = false
}

variable "ecr_image_tag_mutability" {
  description = "ECR image tag mutability. Use IMMUTABLE for stronger supply-chain safety."
  type        = string
  default     = "IMMUTABLE"

  validation {
    condition     = contains(["MUTABLE", "IMMUTABLE"], var.ecr_image_tag_mutability)
    error_message = "ecr_image_tag_mutability must be MUTABLE or IMMUTABLE."
  }
}

variable "app_readonly_root_filesystem" {
  description = "Enable a read-only root filesystem for the sample app container."
  type        = bool
  default     = true
}

variable "firelens_readonly_root_filesystem" {
  description = "Enable a read-only root filesystem for the FireLens container. The aws-for-fluent-bit init image writes startup files, so keep this false unless the selected image and mounts are verified."
  type        = bool
  default     = false
}

variable "firelens_container_user" {
  description = "Optional user for the FireLens container. Leave null to use the selected image default."
  type        = string
  default     = null
}

variable "allowed_ingress_cidrs" {
  description = "CIDR ranges allowed to access the demo HTTP endpoint."
  type        = list(string)
  default     = []
}

variable "tags" {
  description = "Additional resource tags."
  type        = map(string)
  default     = {}
}
