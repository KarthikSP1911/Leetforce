terraform {
  required_version = ">= 1.6"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
  }

  # State lives in the single S3 bucket (ADR 0021) under its own key, separate from
  # infra/neon, so a destroy here cannot see, let alone touch, the database project.
  # The bucket name comes from infra/backend.hcl (git-ignored):
  #   terraform init -backend-config=../backend.hcl
  backend "s3" {
    key          = "tfstate/aws.tfstate"
    region       = "ap-south-1"
    encrypt      = true
    use_lockfile = true
  }
}

provider "aws" {
  region = var.region

  default_tags {
    tags = {
      Project   = "leetforce"
      ManagedBy = "terraform"
      Stack     = "infra-aws"
    }
  }
}
