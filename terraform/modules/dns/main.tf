# Cloudflare DNS for silkstrand.io
#
# Manages DNS records pointing to Cloud Run services.
# Cloudflare handles edge TLS termination.

terraform {
  required_providers {
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "~> 4.0"
    }
  }
}

variable "zone_id" {
  description = "Cloudflare zone ID for silkstrand.io"
  type        = string
}

variable "environment" {
  description = "Environment name (stage or prod)"
  type        = string
}

variable "api_cloud_run_url" {
  description = "Cloud Run URL for the API service (e.g., silkstrand-api-xxxxx.run.app)"
  type        = string
  default     = ""
}

variable "web_cloud_run_url" {
  description = "Cloud Run URL for the Web service (e.g., silkstrand-web-xxxxx.run.app)"
  type        = string
  default     = ""
}

locals {
  # Stage uses subdomains with stage- prefix, prod uses bare subdomains
  api_subdomain = var.environment == "prod" ? "api" : "api-stage"
  app_subdomain = var.environment == "prod" ? "app" : "app-stage"
  wss_subdomain = var.environment == "prod" ? "agent" : "agent-stage"
}

# API endpoint: api.silkstrand.io (prod) / api-stage.silkstrand.io (stage)
resource "cloudflare_record" "api" {
  count = var.api_cloud_run_url != "" ? 1 : 0

  zone_id = var.zone_id
  name    = local.api_subdomain
  content = var.api_cloud_run_url
  type    = "CNAME"
  proxied = true
  ttl     = 1 # auto when proxied
}

# Web app: app.silkstrand.io (prod) / app-stage.silkstrand.io (stage)
resource "cloudflare_record" "web" {
  count = var.web_cloud_run_url != "" ? 1 : 0

  zone_id = var.zone_id
  name    = local.app_subdomain
  content = var.web_cloud_run_url
  type    = "CNAME"
  proxied = true
  ttl     = 1
}

# Agent WSS endpoint: agent.silkstrand.io (prod) / agent-stage.silkstrand.io (stage)
# Points to the same API service (WSS is handled by the API server)
resource "cloudflare_record" "agent" {
  count = var.api_cloud_run_url != "" ? 1 : 0

  zone_id = var.zone_id
  name    = local.wss_subdomain
  content = var.api_cloud_run_url
  type    = "CNAME"
  proxied = true
  ttl     = 1
}

output "api_fqdn" {
  value = "${local.api_subdomain}.silkstrand.io"
}

output "app_fqdn" {
  value = "${local.app_subdomain}.silkstrand.io"
}

output "agent_fqdn" {
  value = "${local.wss_subdomain}.silkstrand.io"
}
