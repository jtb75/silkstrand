# Bootstrap — run once manually to create Terraform state backends.
#
# This is intentionally NOT managed by remote state (chicken-and-egg).
# Run locally:
#   cd terraform/bootstrap
#   terraform init
#   terraform apply -var="stage_project=silkstrand-stage" -var="prod_project=silkstrand-prod"
#
# After this, all other Terraform configs use these buckets as their backend.

terraform {
  required_version = ">= 1.7"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}

variable "stage_project" {
  description = "GCP project ID for staging"
  type        = string
}

variable "prod_project" {
  description = "GCP project ID for production"
  type        = string
}

variable "region" {
  description = "GCS bucket region"
  type        = string
  default     = "us-central1"
}

# --- Stage State Bucket ---

provider "google" {
  alias   = "stage"
  project = var.stage_project
}

resource "google_storage_bucket" "tfstate_stage" {
  provider = google.stage

  name     = "${var.stage_project}-tfstate"
  location = var.region
  project  = var.stage_project

  versioning {
    enabled = true
  }

  uniform_bucket_level_access = true

  lifecycle {
    prevent_destroy = true
  }
}

# --- Prod State Bucket ---

provider "google" {
  alias   = "prod"
  project = var.prod_project
}

resource "google_storage_bucket" "tfstate_prod" {
  provider = google.prod

  name     = "${var.prod_project}-tfstate"
  location = var.region
  project  = var.prod_project

  versioning {
    enabled = true
  }

  uniform_bucket_level_access = true

  lifecycle {
    prevent_destroy = true
  }
}

# --- Workload Identity Federation for GitHub Actions ---
# These allow GitHub Actions to authenticate to GCP without service account keys.

# Stage WIF
resource "google_iam_workload_identity_pool" "github_stage" {
  provider = google.stage

  project                   = var.stage_project
  workload_identity_pool_id = "github-actions"
  display_name              = "GitHub Actions"
}

resource "google_iam_workload_identity_pool_provider" "github_stage" {
  provider = google.stage

  project                            = var.stage_project
  workload_identity_pool_id          = google_iam_workload_identity_pool.github_stage.workload_identity_pool_id
  workload_identity_pool_provider_id = "github"
  display_name                       = "GitHub"

  attribute_mapping = {
    "google.subject"       = "assertion.sub"
    "attribute.actor"      = "assertion.actor"
    "attribute.repository" = "assertion.repository"
  }

  oidc {
    issuer_uri = "https://token.actions.githubusercontent.com"
  }
}

resource "google_service_account" "github_actions_stage" {
  provider = google.stage

  project      = var.stage_project
  account_id   = "github-actions"
  display_name = "GitHub Actions CI/CD"
}

resource "google_service_account_iam_member" "wif_stage" {
  provider = google.stage

  service_account_id = google_service_account.github_actions_stage.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "principalSet://iam.googleapis.com/${google_iam_workload_identity_pool.github_stage.name}/attribute.repository/jtb75/silkstrand"
}

# Prod WIF
resource "google_iam_workload_identity_pool" "github_prod" {
  provider = google.prod

  project                   = var.prod_project
  workload_identity_pool_id = "github-actions"
  display_name              = "GitHub Actions"
}

resource "google_iam_workload_identity_pool_provider" "github_prod" {
  provider = google.prod

  project                            = var.prod_project
  workload_identity_pool_id          = google_iam_workload_identity_pool.github_prod.workload_identity_pool_id
  workload_identity_pool_provider_id = "github"
  display_name                       = "GitHub"

  attribute_mapping = {
    "google.subject"       = "assertion.sub"
    "attribute.actor"      = "assertion.actor"
    "attribute.repository" = "assertion.repository"
  }

  oidc {
    issuer_uri = "https://token.actions.githubusercontent.com"
  }
}

resource "google_service_account" "github_actions_prod" {
  provider = google.prod

  project      = var.prod_project
  account_id   = "github-actions"
  display_name = "GitHub Actions CI/CD"
}

resource "google_service_account_iam_member" "wif_prod" {
  provider = google.prod

  service_account_id = google_service_account.github_actions_prod.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "principalSet://iam.googleapis.com/${google_iam_workload_identity_pool.github_prod.name}/attribute.repository/jtb75/silkstrand"
}

# --- Outputs ---

output "stage_tfstate_bucket" {
  value = google_storage_bucket.tfstate_stage.name
}

output "prod_tfstate_bucket" {
  value = google_storage_bucket.tfstate_prod.name
}

output "stage_wif_provider" {
  value = google_iam_workload_identity_pool_provider.github_stage.name
}

output "prod_wif_provider" {
  value = google_iam_workload_identity_pool_provider.github_prod.name
}

output "stage_wif_sa" {
  value = google_service_account.github_actions_stage.email
}

output "prod_wif_sa" {
  value = google_service_account.github_actions_prod.email
}
