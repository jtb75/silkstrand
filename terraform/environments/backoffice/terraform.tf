# SilkStrand — Backoffice Environment
#
# Terraform/OpenTofu configuration and backend.

terraform {
  required_version = ">= 1.7"

  backend "gcs" {
    bucket = "silkstrand-backoffice-tfstate"
    prefix = "terraform/state"
  }

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.0"
    }
  }
}

provider "google" {
  project = var.project_id
  region  = var.region
}
