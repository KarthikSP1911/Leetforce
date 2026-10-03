terraform {
  required_version = ">= 1.10"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
  }

  # Local state on purpose: this stack creates the bucket that holds every other
  # stack's state, so it cannot store its own there (chicken and egg). It holds one
  # bucket, so losing the file costs one `terraform import`. See ADR 0021.
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
      Stack     = "infra-bootstrap"
    }
  }
}
