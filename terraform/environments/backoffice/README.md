# Backoffice Infrastructure

## Prerequisites
1. Create GCP project `silkstrand-backoffice`
2. Enable required APIs (Cloud Run, Cloud SQL, VPC, etc.)
3. Create GCS bucket `silkstrand-backoffice-tfstate` for state
4. Set up Workload Identity Federation for CI/CD

## Deploy
```
tofu init
tofu plan -var-file=terraform.tfvars
tofu apply -var-file=terraform.tfvars
```

## Required Variables
Set in `terraform.tfvars` (not committed):
- `backoffice_api_image`
- `backoffice_web_image`
- `jwt_secret`
- `encryption_key`
