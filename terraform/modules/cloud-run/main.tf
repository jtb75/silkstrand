# Cloud Run module — API service

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

variable "vpc_connector_name" {
  type = string
}

variable "database_url" {
  type      = string
  sensitive = true
}

variable "redis_url" {
  type      = string
  sensitive = true
}

variable "jwt_secret" {
  type      = string
  sensitive = true
}

variable "internal_api_key" {
  description = "Shared secret for the backoffice to call /internal/v1/ routes"
  type        = string
  sensitive   = true
  default     = ""
}

variable "image" {
  description = "Container image to deploy. Use a placeholder for initial creation."
  type        = string
  default     = "gcr.io/cloudrun/hello"
}

variable "allowed_origins" {
  description = "Comma-separated list of allowed CORS / WebSocket origins. Empty disables CORS."
  type        = string
  default     = ""
}

variable "min_instances" {
  type    = number
  default = 0
}

variable "max_instances" {
  type    = number
  default = 2
}

# Service account for Cloud Run
resource "google_service_account" "api" {
  project      = var.project_id
  account_id   = "silkstrand-api-${var.environment}"
  display_name = "SilkStrand API (${var.environment})"
}

# Grant Cloud SQL Client role
resource "google_project_iam_member" "api_cloudsql" {
  project = var.project_id
  role    = "roles/cloudsql.client"
  member  = "serviceAccount:${google_service_account.api.email}"
}

# Grant Secret Manager access
resource "google_project_iam_member" "api_secrets" {
  project = var.project_id
  role    = "roles/secretmanager.secretAccessor"
  member  = "serviceAccount:${google_service_account.api.email}"
}

# Grant GCS read for bundles
resource "google_project_iam_member" "api_storage" {
  project = var.project_id
  role    = "roles/storage.objectViewer"
  member  = "serviceAccount:${google_service_account.api.email}"
}

# Cloud Run service
resource "google_cloud_run_v2_service" "api" {
  project  = var.project_id
  name     = "silkstrand-api"
  location = var.region
  ingress  = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = google_service_account.api.email

    annotations = {
      "run.googleapis.com/vpc-access-connector" = "projects/${var.project_id}/locations/${var.region}/connectors/${var.vpc_connector_name}"
      "run.googleapis.com/vpc-access-egress"    = "private-ranges-only"
      "run.googleapis.com/cpu-throttling"       = "true"
      "run.googleapis.com/startup-cpu-boost"    = "true"
      "client.knative.dev/user-agent"           = "silkstrand-terraform"
    }

    scaling {
      min_instance_count = var.min_instances
      max_instance_count = var.max_instances
    }

    vpc_access {
      connector = "projects/${var.project_id}/locations/${var.region}/connectors/${var.vpc_connector_name}"
      egress    = "PRIVATE_RANGES_ONLY"
    }

    containers {
      image = var.image

      ports {
        container_port = 8080
      }

      env {
        name  = "ENV"
        value = var.environment
      }

      env {
        name  = "DATABASE_URL"
        value = var.database_url
      }

      env {
        name  = "REDIS_URL"
        value = var.redis_url
      }

      env {
        name  = "JWT_SECRET"
        value = var.jwt_secret
      }

      env {
        name  = "INTERNAL_API_KEY"
        value = var.internal_api_key
      }

      env {
        name  = "ALLOWED_ORIGINS"
        value = var.allowed_origins
      }

      resources {
        cpu_idle = true
        limits = {
          cpu    = "1"
          memory = "256Mi"
        }
      }

      startup_probe {
        initial_delay_seconds = 10
        timeout_seconds       = 5
        period_seconds        = 10
        failure_threshold     = 3
        http_get {
          path = "/healthz"
          port = 8080
        }
      }

      liveness_probe {
        timeout_seconds   = 5
        period_seconds    = 30
        failure_threshold = 3
        http_get {
          path = "/healthz"
          port = 8080
        }
      }
    }
  }

  lifecycle {
    # Image is updated via Terraform -var="image=..." during CI/CD deploys.
    # Do NOT use gcloud run deploy — it creates v1 API revisions that conflict
    # with this v2 service's routing.
  }
}

# Allow unauthenticated access (public API)
resource "google_cloud_run_v2_service_iam_member" "public" {
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.api.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}

output "service_url" {
  value = google_cloud_run_v2_service.api.uri
}

output "service_name" {
  value = google_cloud_run_v2_service.api.name
}

# Extract just the hostname for DNS CNAME
output "service_hostname" {
  value = replace(google_cloud_run_v2_service.api.uri, "https://", "")
}
