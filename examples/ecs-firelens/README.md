# ECS FireLens Example

This example runs a Go app on ECS Fargate and sends its JSON logs to Google Cloud Logging through the `stackdriver_wif` Fluent Bit plugin.

This directory is a sample consumer of the plugin. The plugin source and artifact build live in the top-level `plugin/` directory.

## What It Creates

Terraform creates:

- AWS VPC, public subnets, ECS cluster, ECS service, task definition, ECR repositories, and IAM roles
- Google service account, Workload Identity Pool, AWS provider, and Cloud Logging writer IAM binding

The sample uses ECS task role credentials. It does not use a Google service account key.

## Requirements

- Terraform 1.14.x
- Docker with Buildx
- AWS credentials
- Google Cloud credentials for Terraform
- Permission to enable IAM, STS, IAMCredentials, and Cloud Logging APIs in the Google Cloud project

## 1. Create the Infrastructure

`gcp_project_id` is required. Ingress is closed by default. Set `allowed_ingress_cidrs` if you want to call the demo app from your machine.

```bash
cd examples/ecs-firelens/infra
terraform init
terraform apply \
  -var='gcp_project_id=YOUR_GCP_PROJECT_ID' \
  -var='allowed_ingress_cidrs=["YOUR_IP_ADDRESS/32"]' \
  -var='app_image_tag=example-1' \
  -var='firelens_image_tag=example-1'
```

If you use an AWS shared config profile:

```bash
terraform apply \
  -var='gcp_project_id=YOUR_GCP_PROJECT_ID' \
  -var='aws_profile=YOUR_AWS_PROFILE' \
  -var='allowed_ingress_cidrs=["YOUR_IP_ADDRESS/32"]' \
  -var='app_image_tag=example-1' \
  -var='firelens_image_tag=example-1'
```

## 2. Push the App Image

From this directory:

```bash
APP_IMAGE_TAG=example-1 make app-build-push
```

From the repository root:

```bash
APP_IMAGE_TAG=example-1 make example-app-build-push
```

## 3. Build and Push the FireLens Image

This builds `plugin/dist/out_stackdriver_wif.so` first, then builds the sample FireLens image with that artifact.

From this directory:

```bash
FLUENTBIT_IMAGE_TAG=example-1 make fluentbit-build-push
```

From the repository root:

```bash
FLUENTBIT_IMAGE_TAG=example-1 make example-fluentbit-build-push
```

## 4. Redeploy ECS

After pushing both images:

```bash
cd examples/ecs-firelens/infra
terraform apply \
  -var='gcp_project_id=YOUR_GCP_PROJECT_ID' \
  -var='app_image_tag=example-1' \
  -var='firelens_image_tag=example-1'
```

## 5. Test

Call the task public IP:

```bash
curl "http://TASK_PUBLIC_IP:8080/?message=hello"
curl "http://TASK_PUBLIC_IP:8080/health"
```

Read Cloud Logging:

```bash
GCP_PROJECT_ID="$(terraform output -raw gcp_project_id)"
LOG_ID="$(terraform output -raw cloud_logging_log_id)"

gcloud logging read \
  "logName=\"projects/${GCP_PROJECT_ID}/logs/${LOG_ID}\"" \
  --project "${GCP_PROJECT_ID}" \
  --limit 10 \
  --format json
```

## Notes

- Do not commit Terraform state, `terraform.tfvars`, local credentials, or generated `.so` files.
- Keep `allowed_ingress_cidrs` narrow. Avoid `0.0.0.0/0` unless you are intentionally testing public access.
- Terraform uses local state by default in this example. For real deployments, use an encrypted remote backend with access control.
- The sample service runs in public subnets and allows all outbound traffic for simplicity. For real deployments, use private subnets, VPC endpoints or NAT, and explicit egress controls.
- ECR repositories are not force-deleted by default. Set `ecr_force_delete=true` only for disposable test environments.
- ECR tags are immutable by default. Use a new image tag for each rebuild, or set `ecr_image_tag_mutability="MUTABLE"` for quick local iteration.
- The FireLens container runs as a non-root user and both containers use read-only root filesystems by default. Set `firelens_container_user` or `readonly_root_filesystem` only if the selected Fluent Bit image requires it.
- This is a small verification environment. It does not include an ALB, private subnets, NAT, autoscaling, or VPC endpoints.
