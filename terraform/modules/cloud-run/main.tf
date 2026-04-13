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
}

variable "credential_encryption_key" {
  description = "AES-256 key (64 hex chars) for encrypting target credentials at rest. Stored in Secret Manager and mounted via secret_key_ref."
  type        = string
  sensitive   = true
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

# Secret Manager — every sensitive env var is mounted via secret_key_ref so
# its value never appears in `gcloud run services describe` output. The
# Cloud Run service account already has roles/secretmanager.secretAccessor
# (granted below) and so can read all of these.
#
# Rotation: bump the secret_data, terraform apply (creates a new version);
# Cloud Run picks up `latest` on next instance start. Force a roll with a
# new revision if you need immediate effect.

locals {
  # Non-sensitive metadata — safe for for_each. Keys map to the env var
  # name and double as the secret_id suffix (with underscores → dashes).
  api_secret_envs = {
    credential_encryption_key = "CREDENTIAL_ENCRYPTION_KEY"
    database_url              = "DATABASE_URL"
    redis_url                 = "REDIS_URL"
    jwt_secret                = "JWT_SECRET"
    internal_api_key          = "INTERNAL_API_KEY"
  }
  # Sensitive values — looked up by key inside resources, never iterated.
  api_secret_values = {
    credential_encryption_key = var.credential_encryption_key
    database_url              = var.database_url
    redis_url                 = var.redis_url
    jwt_secret                = var.jwt_secret
    internal_api_key          = var.internal_api_key
  }
}

resource "google_secret_manager_secret" "api" {
  for_each  = local.api_secret_envs
  project   = var.project_id
  secret_id = "${replace(each.key, "_", "-")}-${var.environment}"

  replication {
    auto {}
  }
}

resource "google_secret_manager_secret_version" "api" {
  for_each    = local.api_secret_envs
  secret      = google_secret_manager_secret.api[each.key].id
  secret_data = local.api_secret_values[each.key]
}

# Refactor: the credential encryption key was previously declared as a
# standalone resource (google_secret_manager_secret.credential_encryption_key).
# These moved blocks let Terraform re-key the existing state into the new
# for_each map address rather than destroy + recreate the secret.
moved {
  from = google_secret_manager_secret.credential_encryption_key
  to   = google_secret_manager_secret.api["credential_encryption_key"]
}

moved {
  from = google_secret_manager_secret_version.credential_encryption_key
  to   = google_secret_manager_secret_version.api["credential_encryption_key"]
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

    # Agents hold long-lived WebSocket connections; Cloud Run's default
    # 300s request timeout tears them down every 5 minutes and forces
    # agents to reconnect (and pending scan directives published during
    # the reconnect window are lost to Redis pub/sub). 3600s (60 min) is
    # the Cloud Run max for HTTP services and substantially reduces the
    # reconnect cadence.
    timeout = "3600s"

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
        name  = "ALLOWED_ORIGINS"
        value = var.allowed_origins
      }

      dynamic "env" {
        for_each = local.api_secret_envs
        content {
          name = env.value
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.api[env.key].secret_id
              version = "latest"
            }
          }
        }
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

  # Ensure all secret versions exist before Cloud Run tries to mount them.
  # The secret_key_refs above only depend on the secret, not the version.
  depends_on = [google_secret_manager_secret_version.api]

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
