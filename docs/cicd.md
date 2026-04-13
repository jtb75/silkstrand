# CI/CD Strategy

## Branching Strategy

```
feature/xyz ──PR──▶ main ──auto──▶ silkstrand-stage
fix/abc     ──PR──▶ main     │
                              │  git tag v1.x.x
                              ▼
                        silkstrand-prod
```

- No direct commits to `main`
- All changes via `feature/` or `fix/` branches with PR
- PR requires passing CI checks before merge
- Merge to `main` auto-deploys to stage
- Git tag (`v*`) promotes to prod

## GCP Projects

| Environment | Project ID | Purpose |
|-------------|-----------|---------|
| Stage | `silkstrand-stage` | Auto-deploy on merge. Integration testing. |
| Prod | `silkstrand-prod` | Manual promote via git tag. Customer-facing. |

## GitHub Actions Workflows

### `ci.yml` — PR Checks

**Trigger:** Pull request to `main`

| Job | What it does |
|-----|-------------|
| `go-lint-test` | golangci-lint + `go test` for agent and API |
| `web-lint-test` | npm lint, typecheck, test for web |
| `build-images` | Docker build (no push) to verify images build |
| `terraform-plan-stage` | `terraform plan` for stage, posts output as PR comment |
| `terraform-plan-prod` | `terraform plan` for prod, posts output as PR comment |

### `deploy-stage.yml` — Deploy to Stage

**Trigger:** Push to `main` (merge)

| Job | What it does |
|-----|-------------|
| `build-push-api` | Build + push API image to Artifact Registry (tagged `sha-<commit>`) |
| `build-push-web` | Build + push tenant web image to Artifact Registry |
| `build-push-backoffice-api` | Build + push backoffice API image |
| `build-push-backoffice-web` | Build + push backoffice web image |
| `deploy` | `terraform apply` for stage with the new image SHAs |
| `smoke-test` | Hit `/healthz` to verify deployment |

### `deploy-prod.yml` — Deploy to Prod

**Trigger:** Push tag `v*`

| Job | What it does |
|-----|-------------|
| `validate-tag` | Extract version, verify images exist in Artifact Registry for that SHA |
| `deploy` | `terraform apply` for prod with the same image SHAs from stage |
| `smoke-test` | Hit prod `/healthz` to verify deployment |

### `release-agent.yml` — Agent Binary Release

**Trigger:** Push tag `v*` (runs alongside deploy-prod)

| Job | What it does |
|-----|-------------|
| `build-agent` | Cross-compile Go agent for linux/darwin/windows (amd64/arm64) |
| `publish-release` | Attach binaries + checksums to GitHub Release |

## Authentication to GCP

Uses **Workload Identity Federation** (WIF) — no service account keys stored in GitHub.

GitHub Actions authenticates via OIDC token exchange:
1. GitHub issues an OIDC token for the workflow run
2. Token is exchanged with GCP for short-lived credentials
3. Credentials are scoped to the `github-actions` service account in each project

### Required GitHub Repository Variables

| Variable | Description |
|----------|-------------|
| `WIF_PROVIDER_STAGE` | WIF provider resource name for stage project |
| `WIF_SA_STAGE` | Service account email for stage project |
| `WIF_PROVIDER_PROD` | WIF provider resource name for prod project |
| `WIF_SA_PROD` | Service account email for prod project |
| `STAGE_API_URL` | Stage API URL for smoke tests |

These are output by `terraform/bootstrap/main.tf`.

## Container Images

Images are stored in **GCP Artifact Registry** (one repo per environment, since stage builds push to stage's project; prod's deploy reads the same SHA from prod's repo via the workflow's tag-validation step):

```
us-central1-docker.pkg.dev/silkstrand-stage/silkstrand/api:sha-<commit>
us-central1-docker.pkg.dev/silkstrand-stage/silkstrand/api:latest
us-central1-docker.pkg.dev/silkstrand-stage/silkstrand/web:sha-<commit>
us-central1-docker.pkg.dev/silkstrand-stage/silkstrand/backoffice-api:sha-<commit>
us-central1-docker.pkg.dev/silkstrand-stage/silkstrand/backoffice-web:sha-<commit>
```

(Same shape under `silkstrand-prod` for the prod-tagged builds.)

- Images are tagged by git commit SHA — same SHA deploys to stage and prod
- No rebuild for prod promotion — the exact image tested in stage is deployed
- `latest` tag is updated on each push for convenience but never used in deployments

## Terraform State

Remote state stored in GCS (one bucket per environment):

```
gs://silkstrand-stage-tfstate/terraform/state
gs://silkstrand-prod-tfstate/terraform/state
```

State buckets are created by the bootstrap (`terraform/bootstrap/main.tf`).

## Bootstrap Procedure

Run once before any CI/CD pipelines work:

```bash
cd terraform/bootstrap
terraform init
terraform apply \
  -var="stage_project=silkstrand-stage" \
  -var="prod_project=silkstrand-prod"
```

This creates:
1. GCS buckets for Terraform remote state (both environments)
2. Workload Identity Federation pools and providers (both environments)
3. Service accounts for GitHub Actions (both environments)

After bootstrap, copy the output values into GitHub repository variables.

## Release Process

1. Develop on `feature/` or `fix/` branch
2. Open PR to `main` — CI runs, Terraform plans posted as comments
3. Review and merge — auto-deploys to `silkstrand-stage`
4. Verify in stage
5. Tag release: `git tag v1.0.0 && git push origin v1.0.0`
6. Prod deploy runs, GitHub Release created with agent binaries
