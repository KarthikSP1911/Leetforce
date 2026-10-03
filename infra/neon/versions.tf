terraform {
  required_version = ">= 1.6"

  required_providers {
    neon = {
      source  = "kislerdm/neon"
      version = "~> 0.6"
    }
  }

  # State lives in the single S3 bucket (ADR 0021) under its own key: this stack
  # holds the database, so its state is kept apart from infra/aws and is never
  # touched by arena.sh. Bucket name comes from infra/backend.hcl (git-ignored).
  backend "s3" {
    key          = "tfstate/neon.tfstate"
    region       = "ap-south-1"
    encrypt      = true
    use_lockfile = true
  }
}

# The API key comes from the NEON_API_KEY environment variable, never a file.
provider "neon" {}
