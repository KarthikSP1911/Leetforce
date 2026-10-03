terraform {
  required_version = ">= 1.6"

  required_providers {
    neon = {
      source  = "kislerdm/neon"
      version = "~> 0.6"
    }
  }

  # Local state on purpose (ADR 0020): this stack holds the database, so its
  # state is kept apart from infra/aws and is never touched by arena.sh.
  backend "local" {
    path = "terraform.tfstate"
  }
}

# The API key comes from the NEON_API_KEY environment variable, never a file.
provider "neon" {}
