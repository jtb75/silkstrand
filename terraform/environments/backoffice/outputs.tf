# --- Cloud Run URLs ---

output "backoffice_api_url" {
  description = "Backoffice API Cloud Run URL"
  value       = module.backoffice_api.service_url
}

output "backoffice_web_url" {
  description = "Backoffice web frontend Cloud Run URL"
  value       = google_cloud_run_v2_service.backoffice_web.uri
}

# --- Database ---

output "database_instance_name" {
  description = "Cloud SQL instance name"
  value       = module.database.instance_name
}

output "database_connection_name" {
  description = "Cloud SQL connection name (project:region:instance)"
  value       = module.database.instance_connection_name
}

output "database_private_ip" {
  description = "Cloud SQL private IP address"
  value       = module.database.private_ip
}

output "database_url" {
  description = "Full database connection URL"
  value       = module.database.database_url
  sensitive   = true
}

# --- Networking ---

output "network_id" {
  description = "VPC network ID"
  value       = module.networking.network_id
}

output "vpc_connector_name" {
  description = "Serverless VPC Access connector name"
  value       = module.networking.vpc_connector_name
}
