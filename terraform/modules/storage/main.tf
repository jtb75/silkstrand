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
