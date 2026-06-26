terraform {
  required_version = ">= 1.15.7, < 1.16.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 6.41.0, < 7.0.0"
    }

    google = {
      source  = "hashicorp/google"
      version = ">= 7.28.0, < 8.0.0"
    }
  }
}

provider "aws" {
  region  = var.aws_region
  profile = var.aws_profile
}

provider "google" {
  project = var.gcp_project_id
  region  = local.gcp_region
}
