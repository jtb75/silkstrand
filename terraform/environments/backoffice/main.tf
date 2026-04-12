# SilkStrand — Backoffice Environment
#
# Internal backoffice for managing tenants, agents, and compliance bundles.
# Runs in its own GCP project (silkstrand-backoffice).

# --- Networking ---
module "networking" {
  source = "../../modules/networking"

  project_id  = var.project_id
  region      = var.region
  environment = "backoffice"
}

# --- Database ---
#
# NOTE: The database module hardcodes the database name as "silkstrand" and the
# user as "silkstrand". Since this runs in its own GCP project, that's acceptable
# — there's no naming collision. If a distinct DB name (e.g. "silkstrand_backoffice")
# is needed, the database module would need a `db_name` variable added. Do NOT
# modify the shared module without coordinating with stage/prod.
module "database" {
  source = "../../modules/database"

  project_id                  = var.project_id
  region                      = var.region
  environment                 = "backoffice"
  network_id                  = module.networking.network_id
  tier                        = "db-f1-micro"
  private_services_connection = module.networking.private_services_connection
}

# --- Cloud Run: Backoffice API ---
#
# NOTE: The cloud-run module is purpose-built for the SilkStrand API service.
# It hardcodes the service name ("silkstrand-api"), container port (8080),
# environment variables (DATABASE_URL, REDIS_URL, JWT_SECRET), and service
# account name. For the backoffice API, this means:
#
#   - The service will be named "silkstrand-api" (acceptable since it's in its
#     own GCP project — no collision with stage/prod).
#   - The container MUST listen on port 8080.
#   - redis_url is required even if not used — pass an empty string or dummy value
#     if the backoffice doesn't use Redis.
#   - ENCRYPTION_KEY must be passed via a separate mechanism (e.g. Secret Manager
#     reference, or the module needs an `extra_env_vars` map added).
#
# To fully support the backoffice, the cloud-run module should be extended with:
#   - variable "service_name" (to override the default "silkstrand-api")
#   - variable "container_port" (to override 8080)
#   - variable "extra_env_vars" (map of additional env vars)
# These changes are backward-compatible but should be coordinated with stage/prod.
module "backoffice_api" {
  source = "../../modules/cloud-run"

  project_id         = var.project_id
  region             = var.region
  environment        = "backoffice"
  image              = var.backoffice_api_image
  vpc_connector_name = module.networking.vpc_connector_name
  database_url       = module.database.database_url
  redis_url          = "" # Backoffice does not use Redis pub/sub
  jwt_secret         = var.jwt_secret
  min_instances      = 0
  max_instances      = 2
}

# --- Cloud Run: Backoffice Web Frontend ---
#
# NOTE: The cloud-run module cannot be reused directly for the web frontend
# because it hardcodes the API service name, service account, IAM roles, env
# vars, and health check paths. Using it a second time in the same project would
# also cause Terraform resource name collisions (e.g. two "google_service_account.api").
#
# Options:
#   1. Create a dedicated "cloud-run-web" module for static/SPA frontends.
#   2. Extend the existing cloud-run module with a "service_name" variable and
#      make env vars/probes configurable.
#
# For now, the web frontend is defined inline below.

resource "google_service_account" "backoffice_web" {
  project      = var.project_id
  account_id   = "backoffice-web"
  display_name = "Backoffice Web Frontend"
}

resource "google_cloud_run_v2_service" "backoffice_web" {
  project  = var.project_id
  name     = "backoffice-web"
  location = var.region
  ingress  = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = google_service_account.backoffice_web.email

    scaling {
      min_instance_count = 0
      max_instance_count = 2
    }

    containers {
      image = var.backoffice_web_image

      ports {
        container_port = 80
      }

      resources {
        cpu_idle = true
        limits = {
          cpu    = "1"
          memory = "256Mi"
        }
      }

      startup_probe {
        http_get {
          path = "/"
        }
        initial_delay_seconds = 5
        period_seconds        = 3
        failure_threshold     = 10
      }

      liveness_probe {
        http_get {
          path = "/"
        }
        period_seconds = 30
      }
    }
  }

  lifecycle {
    ignore_changes = [
      template[0].containers[0].image, # Image updated by CI/CD, not Terraform
    ]
  }
}

# Allow unauthenticated access to the web frontend
resource "google_cloud_run_v2_service_iam_member" "backoffice_web_public" {
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.backoffice_web.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}
