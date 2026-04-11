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
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "~> 4.0"
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

provider "cloudflare" {
  api_token = var.cloudflare_api_token
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

variable "cloudflare_api_token" {
  description = "Cloudflare API token for DNS management"
  type        = string
  sensitive   = true
}

variable "cloudflare_zone_id" {
  description = "Cloudflare zone ID for silkstrand.io"
  type        = string
}

variable "redis_url" {
  description = "Upstash Redis URL"
  type        = string
  sensitive   = true
}

variable "jwt_secret" {
  description = "JWT signing secret"
  type        = string
  sensitive   = true
}

# --- Networking ---
module "networking" {
  source = "../../modules/networking"

  project_id  = var.project_id
  region      = var.region
  environment = "prod"
}

# --- Database ---
module "database" {
  source = "../../modules/database"

  project_id                  = var.project_id
  region                      = var.region
  environment                 = "prod"
  network_id                  = module.networking.network_id
  tier                        = "db-f1-micro" # Upgrade when needed
  private_services_connection = module.networking.private_services_connection
}

# --- Storage ---
module "storage" {
  source = "../../modules/storage"

  project_id  = var.project_id
  region      = var.region
  environment = "prod"
}

# --- Cloud Run API ---
module "cloud_run" {
  source = "../../modules/cloud-run"

  project_id         = var.project_id
  region             = var.region
  environment        = "prod"
  vpc_connector_name = module.networking.vpc_connector_name
  database_url       = module.database.database_url
  redis_url          = var.redis_url
  jwt_secret         = var.jwt_secret
  min_instances      = 0
  max_instances      = 5
}

# --- DNS ---
module "dns" {
  source = "../../modules/dns"

  zone_id           = var.cloudflare_zone_id
  environment       = "prod"
  api_cloud_run_url = module.cloud_run.service_hostname
}
