# Networking module — VPC, private services access, serverless VPC connector

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

# VPC Network
resource "google_compute_network" "main" {
  project                 = var.project_id
  name                    = "silkstrand-${var.environment}"
  auto_create_subnetworks = false
}

# Subnet for the VPC connector
resource "google_compute_subnetwork" "main" {
  project       = var.project_id
  name          = "silkstrand-${var.environment}-subnet"
  ip_cidr_range = "10.0.0.0/24"
  region        = var.region
  network       = google_compute_network.main.id
}

# Private services access — reserve IP range for Cloud SQL
resource "google_compute_global_address" "private_services" {
  project       = var.project_id
  name          = "silkstrand-${var.environment}-private-ip"
  purpose       = "VPC_PEERING"
  address_type  = "INTERNAL"
  prefix_length = 16
  network       = google_compute_network.main.id
}

resource "google_service_networking_connection" "private_services" {
  network                 = google_compute_network.main.id
  service                 = "servicenetworking.googleapis.com"
  reserved_peering_ranges = [google_compute_global_address.private_services.name]
}

# Serverless VPC Access connector — allows Cloud Run to reach Cloud SQL
resource "google_vpc_access_connector" "main" {
  project       = var.project_id
  name          = "silkstrand-${var.environment}"
  region        = var.region
  ip_cidr_range = "10.8.0.0/28"
  network       = google_compute_network.main.name

  min_instances = 2
  max_instances = 3
}

output "network_id" {
  value = google_compute_network.main.id
}

output "network_name" {
  value = google_compute_network.main.name
}

output "vpc_connector_name" {
  value = google_vpc_access_connector.main.name
}

output "private_services_connection" {
  value = google_service_networking_connection.private_services
}
