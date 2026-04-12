variable "project_id" {
  description = "GCP project ID for the backoffice"
  type        = string
  default     = "silkstrand-backoffice"
}

variable "region" {
  description = "GCP region"
  type        = string
  default     = "us-central1"
}

variable "backoffice_api_image" {
  description = "Backoffice API container image"
  type        = string
}

variable "backoffice_web_image" {
  description = "Backoffice web frontend container image"
  type        = string
}

variable "jwt_secret" {
  description = "JWT signing secret for admin auth"
  type        = string
  sensitive   = true
}

variable "encryption_key" {
  description = "AES-256 encryption key for DC API keys (hex)"
  type        = string
  sensitive   = true
}
