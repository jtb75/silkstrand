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
  description = "Upstash Redis URL for DC API pub/sub"
  type        = string
  sensitive   = true
}

variable "jwt_secret" {
  description = "JWT signing secret for DC API"
  type        = string
  sensitive   = true
}

variable "internal_api_key" {
  description = "API key for backoffice to access DC internal API"
  type        = string
  sensitive   = true
}

variable "credential_encryption_key" {
  description = "AES-256 key (64 hex chars) for credential encryption at rest. Stored in Secret Manager."
  type        = string
  sensitive   = true
}

variable "backoffice_jwt_secret" {
  description = "JWT signing secret for backoffice admin auth"
  type        = string
  sensitive   = true
}

variable "backoffice_encryption_key" {
  description = "AES-256 encryption key for DC API keys stored in backoffice DB (64 hex chars)"
  type        = string
  sensitive   = true
}

variable "backoffice_api_image" {
  description = "Backoffice API container image"
  type        = string
  default     = "gcr.io/cloudrun/hello"
}

variable "web_image" {
  description = "Tenant web frontend container image"
  type        = string
  default     = "gcr.io/cloudrun/hello"
}

variable "backoffice_web_image" {
  description = "Backoffice web frontend container image"
  type        = string
  default     = "gcr.io/cloudrun/hello"
}

variable "bootstrap_admin_email" {
  description = "Email for the bootstrap admin user (only used on first startup when no admins exist)"
  type        = string
  default     = ""
}

variable "bootstrap_admin_password" {
  description = "Password for the bootstrap admin user"
  type        = string
  sensitive   = true
  default     = ""
}

variable "tenant_jwt_secret" {
  description = "HS256 secret for tenant end-user JWTs (shared between backoffice and every DC API)"
  type        = string
  sensitive   = true
}

variable "resend_api_key" {
  description = "Resend API key for sending transactional emails (invites, password resets)"
  type        = string
  sensitive   = true
  default     = ""
}

variable "from_email" {
  description = "From address for transactional email"
  type        = string
  default     = "SilkStrand <noreply@silkstrand.io>"
}

variable "tenant_web_url" {
  description = "Public base URL of the tenant frontend, used to build invite / reset links"
  type        = string
  default     = ""
}

variable "image" {
  description = "Container image for the API (passed from CI on deploy)"
  type        = string
  default     = "gcr.io/cloudrun/hello"
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
variable "github_actions_sa_email" {
  description = "Service account email used by GitHub Actions (WIF). Granted write access to the agent-releases bucket."
  type        = string
  default     = ""
}

module "storage" {
  source = "../../modules/storage"

  project_id                   = var.project_id
  region                       = var.region
  environment                  = "prod"
  create_agent_releases_bucket = true
  agent_releases_writers = (
    var.github_actions_sa_email != ""
    ? ["serviceAccount:${var.github_actions_sa_email}"]
    : []
  )
}

# --- Cloud Run API ---
module "cloud_run" {
  source = "../../modules/cloud-run"

  project_id                = var.project_id
  region                    = var.region
  environment               = "prod"
  image                     = var.image
  vpc_connector_name        = module.networking.vpc_connector_name
  database_url              = module.database.database_url
  redis_url                 = var.redis_url
  jwt_secret                = var.jwt_secret
  internal_api_key          = var.internal_api_key
  credential_encryption_key = var.credential_encryption_key
  allowed_origins           = var.tenant_web_url
  min_instances             = 0
  max_instances             = 5
}

# --- DNS ---
module "dns" {
  source = "../../modules/dns"

  zone_id           = var.cloudflare_zone_id
  environment       = "prod"
  api_cloud_run_url = module.cloud_run.service_hostname
}

# --- Backoffice ---
#
# The backoffice runs in the same GCP project as prod. It uses a second database
# on the same Cloud SQL instance (no extra instance cost) and its own Cloud Run
# services. One backoffice manages all data centers (stage, prod, future regions).

# Second database on the existing Cloud SQL instance for backoffice data
resource "google_sql_database" "backoffice" {
  project  = var.project_id
  instance = module.database.instance_name
  name     = "silkstrand_backoffice"
}

# Note: The backoffice database URL uses the same user/password as the DC database
# (same Cloud SQL instance, different database name). The DATABASE_URL env var in
# the backoffice API Cloud Run service references the private IP directly.

# Backoffice API service account
resource "google_service_account" "backoffice_api" {
  project      = var.project_id
  account_id   = "backoffice-api"
  display_name = "Backoffice API"
}

resource "google_project_iam_member" "backoffice_api_cloudsql" {
  project = var.project_id
  role    = "roles/cloudsql.client"
  member  = "serviceAccount:${google_service_account.backoffice_api.email}"
}

resource "google_project_iam_member" "backoffice_api_secrets" {
  project = var.project_id
  role    = "roles/secretmanager.secretAccessor"
  member  = "serviceAccount:${google_service_account.backoffice_api.email}"
}

# Backoffice API Cloud Run service
resource "google_cloud_run_v2_service" "backoffice_api" {
  project  = var.project_id
  name     = "backoffice-api"
  location = var.region
  ingress  = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = google_service_account.backoffice_api.email

    scaling {
      min_instance_count = 0
      max_instance_count = 2
    }

    vpc_access {
      connector = "projects/${var.project_id}/locations/${var.region}/connectors/${module.networking.vpc_connector_name}"
      egress    = "PRIVATE_RANGES_ONLY"
    }

    containers {
      image = var.backoffice_api_image

      ports {
        container_port = 8081
      }

      env {
        name  = "ENV"
        value = "production"
      }

      # Cloud Run sets PORT automatically from container_port; cannot be set as env var

      env {
        name  = "DATABASE_URL"
        value = "postgres://${module.database.database_user}:${module.database.database_password}@${module.database.private_ip}:5432/silkstrand_backoffice?sslmode=disable"
      }

      env {
        name  = "JWT_SECRET"
        value = var.backoffice_jwt_secret
      }

      env {
        name  = "ENCRYPTION_KEY"
        value = var.backoffice_encryption_key
      }

      # Bootstrap admin on first startup. After the first admin exists,
      # these are ignored. Leave blank/unset to disable bootstrap.
      env {
        name  = "BOOTSTRAP_ADMIN_EMAIL"
        value = var.bootstrap_admin_email
      }

      env {
        name  = "BOOTSTRAP_ADMIN_PASSWORD"
        value = var.bootstrap_admin_password
      }

      env {
        name  = "TENANT_JWT_SECRET"
        value = var.tenant_jwt_secret
      }

      env {
        name  = "RESEND_API_KEY"
        value = var.resend_api_key
      }

      env {
        name  = "FROM_EMAIL"
        value = var.from_email
      }

      env {
        # Passed explicitly from the deploy workflow to avoid a Cloud Run
        # service dependency cycle (backoffice → web → backoffice).
        name  = "TENANT_WEB_URL"
        value = var.tenant_web_url
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
          path = "/healthz"
        }
        initial_delay_seconds = 5
        period_seconds        = 3
        failure_threshold     = 10
      }

      liveness_probe {
        http_get {
          path = "/healthz"
        }
        period_seconds = 30
      }
    }
  }

  lifecycle {
    # Image updated via Terraform -var from CI deploys
  }
}

resource "google_cloud_run_v2_service_iam_member" "backoffice_api_public" {
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.backoffice_api.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}

# Backoffice Web Frontend
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

      env {
        name  = "BACKOFFICE_API_URL"
        value = google_cloud_run_v2_service.backoffice_api.uri
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
    # Image updated via Terraform -var from CI deploys
  }
}

resource "google_cloud_run_v2_service_iam_member" "backoffice_web_public" {
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.backoffice_web.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}

# Tenant Web Frontend (silkstrand-web) — public UI served at the tenant URL.
# Uses the same nginx+envsubst pattern as backoffice-web.
resource "google_service_account" "web" {
  project      = var.project_id
  account_id   = "silkstrand-web"
  display_name = "SilkStrand Tenant Web"
}

resource "google_cloud_run_v2_service" "web" {
  project  = var.project_id
  name     = "silkstrand-web"
  location = var.region
  ingress  = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = google_service_account.web.email

    scaling {
      min_instance_count = 0
      max_instance_count = 2
    }

    containers {
      image = var.web_image

      ports {
        container_port = 80
      }

      env {
        name  = "SILKSTRAND_API_URL"
        value = module.cloud_run.service_url
      }

      env {
        name  = "BACKOFFICE_API_URL"
        value = google_cloud_run_v2_service.backoffice_api.uri
      }

      resources {
        cpu_idle = true
        limits = {
          cpu    = "1"
          memory = "256Mi"
        }
      }

      startup_probe {
        http_get { path = "/" }
        initial_delay_seconds = 5
        period_seconds        = 3
        failure_threshold     = 10
      }

      liveness_probe {
        http_get { path = "/" }
        period_seconds = 30
      }
    }
  }

  lifecycle {
    # Image updated via Terraform -var from CI deploys
  }
}

resource "google_cloud_run_v2_service_iam_member" "web_public" {
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.web.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}

# --- Backoffice Outputs ---
output "backoffice_api_url" {
  value = google_cloud_run_v2_service.backoffice_api.uri
}

output "backoffice_web_url" {
  value = google_cloud_run_v2_service.backoffice_web.uri
}

output "web_url" {
  value = google_cloud_run_v2_service.web.uri
}
