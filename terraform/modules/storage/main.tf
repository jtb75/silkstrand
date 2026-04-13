# Storage module — GCS bucket for compliance bundles

terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}

variable "project_id" {
  type = string
}

variable "region" {
  type = string
}

variable "environment" {
  type = string
}

resource "google_storage_bucket" "bundles" {
  project  = var.project_id
  name     = "silkstrand-${var.environment}-bundles"
  location = var.region

  versioning {
    enabled = true
  }

  uniform_bucket_level_access = true
}

output "bucket_name" {
  value = google_storage_bucket.bundles.name
}

output "bucket_url" {
  value = google_storage_bucket.bundles.url
}

# --- Agent releases bucket (publicly readable) ---
# Hosts silkstrand-agent binaries + install.sh so customers can pull them
# without a Silkstrand login. The agent source is OSS so the binaries
# aren't secret. Only the prod env creates this bucket — stage doesn't
# need it (agents connect to whichever DC is configured).

resource "google_storage_bucket" "agent_releases" {
  count = var.create_agent_releases_bucket ? 1 : 0

  project  = var.project_id
  name     = "silkstrand-agent-releases"
  location = var.region

  uniform_bucket_level_access = true

  versioning {
    enabled = true
  }
}

resource "google_storage_bucket_iam_member" "agent_releases_public" {
  count = var.create_agent_releases_bucket ? 1 : 0

  bucket = google_storage_bucket.agent_releases[0].name
  role   = "roles/storage.objectViewer"
  member = "allUsers"
}

resource "google_storage_bucket_iam_member" "agent_releases_writer" {
  for_each = var.create_agent_releases_bucket ? toset(var.agent_releases_writers) : toset([])

  bucket = google_storage_bucket.agent_releases[0].name
  role   = "roles/storage.objectAdmin"
  member = each.value
}

variable "agent_releases_writers" {
  description = "Member identifiers (e.g. serviceAccount:github-actions@...) granted write access to the agent-releases bucket."
  type        = list(string)
  default     = []
}

variable "create_agent_releases_bucket" {
  description = "Whether to create the public agent-releases bucket. Set true in prod only."
  type        = bool
  default     = false
}

output "agent_releases_bucket_name" {
  value = var.create_agent_releases_bucket ? google_storage_bucket.agent_releases[0].name : ""
}

output "agent_releases_base_url" {
  value = var.create_agent_releases_bucket ? "https://storage.googleapis.com/${google_storage_bucket.agent_releases[0].name}" : ""
}
