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
  description = "Cloud Run hostname for the API service (unused — domain mapping uses ghs.googlehosted.com)"
  type        = string
  default     = ""
}

variable "web_cloud_run_url" {
  description = "Cloud Run hostname for the Web service"
  type        = string
  default     = ""
}

locals {
  api_subdomain = var.environment == "prod" ? "api" : "api-stage"
  app_subdomain = var.environment == "prod" ? "app" : "app-stage"
  wss_subdomain = var.environment == "prod" ? "agent" : "agent-stage"
}

# API endpoint: api.silkstrand.io (prod) / api-stage.silkstrand.io (stage)
# Points to ghs.googlehosted.com for Cloud Run domain mapping (Google-managed TLS)
resource "cloudflare_record" "api" {
  zone_id = var.zone_id
  name    = local.api_subdomain
  content = "ghs.googlehosted.com"
  type    = "CNAME"
  proxied = false
  ttl     = 300
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
# Points to ghs.googlehosted.com for Cloud Run domain mapping (Google-managed TLS)
resource "cloudflare_record" "agent" {
  zone_id = var.zone_id
  name    = local.wss_subdomain
  content = "ghs.googlehosted.com"
  type    = "CNAME"
  proxied = false
  ttl     = 300
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
