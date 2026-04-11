# SilkStrand — Prod Environment
#
# Applied manually via git tag (v*) trigger in GitHub Actions.

terraform {
  required_version = ">= 1.7"

  backend "gcs" {
    bucket = "silkstrand-prod-tfstate"
    prefix = "terraform/state"
  }

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}

provider "google" {
  project = var.project_id
  region  = var.region
}

variable "project_id" {
  description = "GCP project ID"
  type        = string
  default     = "silkstrand-prod"
}

variable "region" {
  description = "GCP region"
  type        = string
  default     = "us-central1"
}

# Modules will be added as we build out infrastructure:
# - cloud-run (API + Web services)
# - cloud-sql (PostgreSQL)
# - gcs (bundle storage)
# - networking (VPC, firewall)
# - iam (service accounts, roles)
