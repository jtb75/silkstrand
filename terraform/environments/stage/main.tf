# SilkStrand — Stage Environment
#
# Auto-applied on merge to main via GitHub Actions.

terraform {
  required_version = ">= 1.7"

  backend "gcs" {
    bucket = "silkstrand-stage-tfstate"
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
  default     = "silkstrand-stage"
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
