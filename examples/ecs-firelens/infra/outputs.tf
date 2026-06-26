output "app_repository_url" {
  description = "ECR repository URL for the Go app image."
  value       = aws_ecr_repository.app.repository_url
}

output "fluentbit_repository_url" {
  description = "ECR repository URL for the custom Fluent Bit image."
  value       = aws_ecr_repository.fluentbit.repository_url
}

output "app_image_uri" {
  description = "Image URI consumed by the ECS app container."
  value       = local.app_image
}

output "firelens_image_uri" {
  description = "Custom Fluent Bit image URI consumed by the ECS log router container."
  value       = local.firelens_image
}

output "ecs_cluster_name" {
  description = "ECS cluster name."
  value       = aws_ecs_cluster.this.name
}

output "ecs_service_name" {
  description = "ECS service name."
  value       = aws_ecs_service.this.name
}

output "ecs_task_role_arn" {
  description = "ECS task role ARN used as the AWS WIF principal."
  value       = aws_iam_role.task.arn
}

output "cloud_logging_log_id" {
  description = "Cloud Logging log ID."
  value       = local.cloud_logging_log_id
}

output "gcp_project_id" {
  description = "Google Cloud project ID used by the demo."
  value       = var.gcp_project_id
}

output "gcp_project_number" {
  description = "Google Cloud project number used in WIF config."
  value       = data.google_project.current.number
}

output "gcp_service_account_email" {
  description = "Google service account impersonated by FireLens through WIF."
  value       = google_service_account.firelens.email
}

output "wif_pool_id" {
  description = "Workload Identity Pool ID."
  value       = google_iam_workload_identity_pool.aws.workload_identity_pool_id
}

output "wif_provider_id" {
  description = "Workload Identity Pool Provider ID."
  value       = google_iam_workload_identity_pool_provider.aws.workload_identity_pool_provider_id
}
