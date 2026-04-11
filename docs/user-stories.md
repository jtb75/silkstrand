# SilkStrand User Stories

## Roles

- **Platform Admin**: Customer-side administrator who deploys and manages agents
- **User**: Customer-side user who configures targets, runs scans, and reviews results
- **Bundle Author**: Internal or third-party author who creates compliance bundles
- **Developer**: SilkStrand developer building and operating the platform

---

## Epic 1: Agent Connectivity

### 1.1 Agent Deployment
**As a** platform admin  
**I want to** deploy a SilkStrand agent in my environment  
**So that** the SaaS platform can orchestrate scans in my private network  

**Acceptance Criteria:**
- Download a single binary or container image
- Provide a registration token during setup
- Agent registers with the SaaS platform and appears in the dashboard
- Agent requires only outbound HTTPS (port 443)

### 1.2 Agent Health Monitoring
**As a** platform admin  
**I want to** see the health status of my agents in the dashboard  
**So that** I know if an agent goes offline or has issues  

**Acceptance Criteria:**
- Dashboard shows agent status: connected, disconnected, error
- Last heartbeat timestamp is visible
- Agent version is displayed
- Alert when an agent has been disconnected for more than 5 minutes

### 1.3 WSS Tunnel Establishment
**As an** agent  
**I want to** establish an outbound WSS tunnel to the SaaS platform  
**So that** the platform can send directives without inbound firewall rules  

**Acceptance Criteria:**
- Agent connects via WSS over port 443
- Supports HTTP/HTTPS proxy for corporate environments
- Sends heartbeat every 30 seconds
- Reconnects with exponential backoff on disconnect (max 5 minutes)

### 1.4 Agent Auto-Reconnect
**As an** agent  
**I want to** automatically reconnect when the tunnel drops  
**So that** temporary network issues don't require manual intervention  

**Acceptance Criteria:**
- Detects tunnel disconnect within 60 seconds
- Reconnects with exponential backoff (1s, 2s, 4s, ... up to 5m)
- Picks up any pending directives on reconnect
- Logs reconnection attempts for troubleshooting

---

## Epic 2: Target Management

### 2.1 Register Scan Target
**As a** user  
**I want to** register a scan target (database, IP, CIDR, cloud account)  
**So that** I can run compliance scans against it  

**Acceptance Criteria:**
- Specify target type: database, host, CIDR, cloud account
- For databases: host, port, database name, engine (PostgreSQL, MySQL)
- For hosts: IP address or hostname
- For CIDR: network range
- Assign target to an agent (which agent can reach it)

### 2.2 Associate Credentials
**As a** user  
**I want to** associate credentials with a target  
**So that** the agent can authenticate when scanning  

**Acceptance Criteria:**
- MVP: store username/password (encrypted in database)
- Post-MVP: specify vault path (e.g., `vault://secret/data/prod-db`)
- Credentials are never displayed in plaintext after initial entry
- Can update or rotate credentials without recreating the target

### 2.3 Group Targets by Environment
**As a** user  
**I want to** organize targets by environment (production, staging, dev)  
**So that** I can manage scans and results by environment  

**Acceptance Criteria:**
- Assign environment label to each target
- Filter targets by environment in the UI
- Run scans against all targets in an environment

---

## Epic 3: Compliance Bundles

### 3.1 Upload Compliance Bundle
**As a** bundle author  
**I want to** upload a compliance bundle with a manifest  
**So that** agents can execute it against targets  

**Acceptance Criteria:**
- Upload `.tar.gz` bundle via API or UI
- Manifest specifies: name, version, framework, target type, entrypoint
- Bundle is stored in GCS with versioning
- Bundle is signed on upload
- Validation: manifest is well-formed, entrypoint exists, framework is supported

### 3.2 Bundle Caching on Agent
**As an** agent  
**I want to** cache bundles locally after first pull  
**So that** I don't re-download on every scan  

**Acceptance Criteria:**
- Cache bundles by name + version
- Verify signature before execution (even from cache)
- Invalidate cache entry when a new version is available
- Configurable cache size limit

### 3.3 Polyglot Framework Support
**As the** platform  
**I want to** support multiple assessment frameworks  
**So that** bundle authors can use the best tool for each benchmark  

**Acceptance Criteria:**
- Support runners for: Python, OVAL (OpenSCAP), Rego (OPA), Perl
- Runner selection based on manifest `framework` field
- All runners produce output conforming to the standard results schema
- Runner failures are captured and reported (timeout, crash, invalid output)

---

## Epic 4: Scan Execution

### 4.1 Trigger On-Demand Scan
**As a** user  
**I want to** trigger a scan of a target against a specific benchmark  
**So that** I can assess compliance on demand  

**Acceptance Criteria:**
- Select target and benchmark from the UI
- Scan starts within 10 seconds of triggering
- Scan record created with status PENDING → RUNNING → COMPLETED/FAILED
- User sees immediate feedback that the scan was queued

### 4.2 Directive Dispatch
**As the** platform  
**I want to** dispatch scan directives to the correct agent  
**So that** the right agent executes the scan  

**Acceptance Criteria:**
- Determine which agent is assigned to the target
- Publish directive to agent's channel via Upstash Redis
- Handle case where agent is disconnected (queue directive, mark scan as WAITING)
- Timeout scan if agent doesn't acknowledge within 5 minutes

### 4.3 Bundle Execution on Agent
**As an** agent  
**I want to** execute a compliance bundle against a target  
**So that** I can assess compliance locally  

**Acceptance Criteria:**
- Pull bundle from GCS if not cached
- Read manifest and select runner
- Inject credentials via environment variables
- Execute with configurable timeout (default: 10 minutes)
- Capture structured JSON output
- Return results over WSS

### 4.4 Real-Time Scan Progress
**As a** user  
**I want to** see scan progress in real-time  
**So that** I know what's happening without refreshing the page  

**Acceptance Criteria:**
- Scan status updates appear in UI without page refresh
- Show: queued → running → completed/failed
- Display elapsed time during scan
- Show error details if scan fails

---

## Epic 5: Results & Reporting

### 5.1 View Scan Results
**As a** user  
**I want to** view scan results with pass/fail per CIS control  
**So that** I understand my compliance posture  

**Acceptance Criteria:**
- Results page shows all controls with status (PASS, FAIL, ERROR, N/A)
- Filter by status (show only failures)
- Sort by severity
- Show evidence for each finding (current value vs. expected)

### 5.2 Scan History & Trending
**As a** user  
**I want to** see scan history and compliance trends over time  
**So that** I can track improvement  

**Acceptance Criteria:**
- List of past scans per target with timestamps and summary scores
- Trend chart: compliance percentage over time
- Diff between two scans: what changed (new failures, resolved findings)

### 5.3 Export Results
**As a** user  
**I want to** export scan results  
**So that** I can share them with auditors or integrate with other tools  

**Acceptance Criteria:**
- Export as JSON (machine-readable, full detail)
- Export as PDF (human-readable report with branding)
- Include: benchmark name, target, date, all controls with status and evidence

### 5.4 Remediation Guidance
**As a** user  
**I want to** see remediation steps for failed controls  
**So that** I know how to fix compliance gaps  

**Acceptance Criteria:**
- Each failed control shows remediation text from the bundle
- Remediation is specific and actionable (not generic CIS text)
- Link to relevant CIS benchmark documentation where applicable

---

## Epic 6: Multi-Tenancy & Authentication

### 6.1 User Authentication
**As a** user  
**I want to** sign up and log in securely  
**So that** I can access my tenant's data  

**Acceptance Criteria:**
- Authentication via Auth0 or Clerk (SSO, email/password)
- JWT-based session management
- Redirect to login on unauthenticated access

### 6.2 Tenant Isolation
**As a** tenant  
**I want to** be confident my data is isolated from other tenants  
**So that** I can trust the platform with sensitive compliance data  

**Acceptance Criteria:**
- All database queries scoped to tenant_id
- API middleware enforces tenant context from JWT
- No API endpoint can access cross-tenant data
- Row-level security in Postgres as defense-in-depth

### 6.3 User & Role Management
**As a** tenant admin  
**I want to** manage users and roles within my organization  
**So that** I can control who does what  

**Acceptance Criteria:**
- Roles: admin, operator (can scan), viewer (read-only)
- Admin can invite users via email
- Admin can change roles and remove users
- Role enforcement on all API endpoints

---

## Epic 7: Infrastructure & DevOps

### 7.1 Terraform-Managed Infrastructure
**As a** developer  
**I want to** deploy all GCP infrastructure via Terraform  
**So that** infrastructure is reproducible and version-controlled  

**Acceptance Criteria:**
- All GCP resources defined in Terraform (Cloud Run, Cloud SQL, GCS, IAM)
- Remote state stored in GCS
- Separate tfvars for dev and prod environments
- CI/CD can run `terraform apply` on merge to main

### 7.2 Local Development Environment
**As a** developer  
**I want to** run the full stack locally  
**So that** I can develop and test without deploying to GCP  

**Acceptance Criteria:**
- `docker-compose.yml` for Postgres and Redis
- Agent connects to local API server
- Web dev server proxies to local API
- Bundles served from local filesystem
- Single command to start everything (`make dev` or similar)

### 7.3 Scale-to-Zero
**As the** platform  
**I want** Cloud Run services to scale to zero when idle  
**So that** costs are minimal during low usage  

**Acceptance Criteria:**
- API server scales to zero with no active connections
- Web frontend served statically (no always-on compute)
- Cold start time under 5 seconds
- Agent reconnects gracefully after API cold start

---

## MVP Scope

For the initial MVP, implement these stories:

1. **Agent**: 1.1, 1.3, 1.4 (deploy, connect, reconnect)
2. **Targets**: 2.1, 2.2 (register targets with basic auth)
3. **Bundles**: 3.1, 3.2, 3.3 (upload, cache, Python runner only)
4. **Scanning**: 4.1, 4.2, 4.3 (trigger scan, dispatch, execute)
5. **Results**: 5.1 (view results)
6. **Auth**: 6.1, 6.2 (login, tenant isolation)
7. **Infra**: 7.1, 7.2 (Terraform, local dev)

**First bundle**: CIS PostgreSQL benchmark (Python-based)

### Deferred to Post-MVP

- Agent health monitoring UI (1.2)
- Environment grouping (2.3)
- OVAL, Rego, Perl runners (3.3 partial)
- Real-time progress streaming (4.4)
- Scan history & trending (5.2)
- Export (5.3)
- Remediation guidance (5.4)
- Role management (6.3)
- Scale-to-zero optimization (7.3)
- Vault integrations for credentials
- CIDR scanning / network discovery
- Custom benchmarks by tenants
- Agent fleet management
- Agent auto-update
