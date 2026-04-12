# Database module — Cloud SQL PostgreSQL with private IP

terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.0"
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

variable "network_id" {
  type = string
}

variable "tier" {
  type    = string
  default = "db-f1-micro"
}

variable "private_services_connection" {
  description = "The private services connection to depend on"
}

# Generate database password
resource "random_password" "db_password" {
  length  = 32
  special = false
}

# Store password in Secret Manager
resource "google_secret_manager_secret" "db_password" {
  project   = var.project_id
  secret_id = "silkstrand-${var.environment}-db-password"

  replication {
    auto {}
  }
}

resource "google_secret_manager_secret_version" "db_password" {
  secret      = google_secret_manager_secret.db_password.id
  secret_data = random_password.db_password.result
}

# Cloud SQL instance
resource "google_sql_database_instance" "main" {
  project          = var.project_id
  name             = "silkstrand-${var.environment}"
  database_version = "POSTGRES_16"
  region           = var.region

  depends_on = [var.private_services_connection]

  settings {
    tier              = var.tier
    availability_type = "ZONAL"
    disk_size         = 10
    disk_autoresize   = true

    ip_configuration {
      ipv4_enabled                                  = false
      private_network                               = var.network_id
      enable_private_path_for_google_cloud_services = true
    }

    backup_configuration {
      enabled    = true
      start_time = "03:00"
    }

    database_flags {
      name  = "max_connections"
      value = "50"
    }
  }

  deletion_protection = false # Set to true for prod
}

# Database
resource "google_sql_database" "main" {
  project  = var.project_id
  instance = google_sql_database_instance.main.name
  name     = "silkstrand"
}

# User
resource "google_sql_user" "main" {
  project  = var.project_id
  instance = google_sql_database_instance.main.name
  name     = "silkstrand"
  password = random_password.db_password.result
}

output "instance_name" {
  value = google_sql_database_instance.main.name
}

output "instance_connection_name" {
  value = google_sql_database_instance.main.connection_name
}

output "private_ip" {
  value = google_sql_database_instance.main.private_ip_address
}

output "database_name" {
  value = google_sql_database.main.name
}

output "database_user" {
  value = google_sql_user.main.name
}

output "database_password" {
  value     = random_password.db_password.result
  sensitive = true
}

output "database_url" {
  value     = "postgres://${google_sql_user.main.name}:${random_password.db_password.result}@${google_sql_database_instance.main.private_ip_address}:5432/${google_sql_database.main.name}?sslmode=disable"
  sensitive = true
}

output "password_secret_id" {
  value = google_secret_manager_secret.db_password.secret_id
}
