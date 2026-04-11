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
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "~> 4.0"
    }
  }
}

provider "google" {
  project = var.project_id
  region  = var.region
}

provider "cloudflare" {
  api_token = var.cloudflare_api_token
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

variable "cloudflare_api_token" {
  description = "Cloudflare API token for DNS management"
  type        = string
  sensitive   = true
}

variable "cloudflare_zone_id" {
  description = "Cloudflare zone ID for silkstrand.io"
  type        = string
}

# DNS records — will be wired up when Cloud Run services are created
module "dns" {
  source = "../../modules/dns"

  zone_id     = var.cloudflare_zone_id
  environment = "stage"

  # These will be populated once Cloud Run modules are added:
  # api_cloud_run_url = module.cloud_run.api_url
  # web_cloud_run_url = module.cloud_run.web_url
}

# TODO: Add modules as we build out infrastructure:
# - cloud-run (API + Web services)
# - cloud-sql (PostgreSQL)
# - gcs (bundle storage)
# - networking (VPC, firewall)
# - iam (service accounts, roles)
