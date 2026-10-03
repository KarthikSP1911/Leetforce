terraform {
  required_version = ">= 1.6"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
  }

  # Local state on purpose (ADR 0020). Separate from infra/neon, so a destroy
  # here cannot see, let alone touch, the database project.
  backend "local" {
    path = "terraform.tfstate"
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
