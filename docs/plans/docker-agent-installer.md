# Docker-mode agent installer

Extend the agent installer to deploy the agent as a Docker container,
with first-class support for scanning the host's own Docker networks.
Motivating use case: a Docker host (macOS or Linux) running many
containers we'd like to discover, where the bridge networks are only
reachable from inside Docker itself.

## Motivating gap

The current `install.sh` drops a native binary + service (systemd or
launchd). That's the right default for bare-metal and VM deployments.
It does not help when:

- The scan target *is* the Docker host's internal networks (e.g.,
  `172.17.0.0/16` and friends). On macOS those subnets aren't even
  routable from the host — the agent must live inside Docker.
- The user wants an agent per customer environment with no root /
  systemd footprint.
- The user wants to opt into scanning specific CIDRs without
  hand-editing `/etc/silkstrand/scan-allowlist.yaml` after install.

After this effort: a single invocation (`install.sh --mode=docker …`)
registers a running containerized agent, wires the allowlist from
flags, and (optionally) attaches the container to every Docker bridge
network on the host.

## Decisions

### D1. One installer, two modes — not two installers

`install.sh` keeps its current flags and default behavior. A new
`--mode=docker` flag (default `--mode=binary`) branches into the
container path. Rationale: one source of truth, shared token handling,
a single thing to document. The docker branch is gated so a user on a
host without Docker gets a clear error, not a cryptic failure.

### D2. Image and versioning

The container uses the same agent image we already publish per tag:

    us-central1-docker.pkg.dev/silkstrand-prod/silkstrand/silkstrand-agent:<tag>

The installer resolves the tag via the same `--version` flag the
binary mode uses (default: `latest`; pinned to `v<installer-version>`
for reproducibility). No new image artifact needed.

### D3. New flags (both modes where sensible)

| Flag | Mode | Effect |
|---|---|---|
| `--mode={binary,docker}` | both | Install path. Default `binary`. |
| `--allow-cidr=CIDR` (repeatable) | both | Appended into the rendered `scan-allowlist.yaml`. In binary mode replaces the manual edit step. |
| `--docker-scan-all-bridges` | docker | Enumerate all Docker bridge networks, filter to RFC1918, add each subnet to the allowlist, and `docker network connect` the agent container to each. |
| `--docker-network=NAME` (repeatable) | docker | Attach to specific named networks. Mutually compatible with `--docker-scan-all-bridges` (set union). |
| `--docker-volume=NAME` | docker | Named volume for `/home/nonroot` (runtimes + creds persist across restarts). Default `silkstrand-agent-runtimes`. |
| `--rate-limit-pps=N` | both | Written into the allowlist. Default 500 (matches current doc). |

### D4. CONNECT-mode naabu by default in docker mode

Containerized agent runs as `nonroot` per the distroless image. Naabu
defaults to SYN scan, which needs `CAP_NET_RAW`. Docker mode exports
`SILKSTRAND_NAABU_SCAN_TYPE=c` so the container stays unprivileged.
Users who want SYN scan can opt in with `--docker-caps=raw` (adds
`--cap-add=NET_RAW --cap-add=NET_ADMIN` plus `SILKSTRAND_NAABU_SCAN_TYPE=`).

### D5. Network attachment is additive, post-start

`docker run --network=X` only accepts one network. The installer starts
the container on the *first* target network, then `docker network
connect`s it to each additional one. Rationale: single code path,
matches how Docker itself layers multi-homing.

If no network flags are given, the container is attached to the
default `bridge` network only — the conservative default.

### D6. Do not mount the Docker socket

The installer does all enumeration (listing networks, resolving their
subnets) on the host side *before* `docker run`. The container itself
never sees `/var/run/docker.sock`. Rationale: mounting the socket is
equivalent to giving the agent root on the host, which is a hard sell
for customer environments. A future `target_type: docker_host` that
wants container-level enrichment (labels, image tags, compose project)
will need its own ADR covering the security model — out of scope here.

### D7. Allowlist lives in a per-agent host file, bind-mounted RO

In-container path stays `/etc/silkstrand/scan-allowlist.yaml` — the
agent code is unaware it's containerized. Host-side path is
namespaced by agent id: `/etc/silkstrand/agents/<agent-id>/scan-allowlist.yaml`.
Bind-mount `…/<agent-id>/scan-allowlist.yaml:/etc/silkstrand/scan-allowlist.yaml:ro`.

Rationale: lets multiple docker agents coexist on the same host
without colliding, leaves binary-mode's canonical path free, and
supports future horizontal scale-out of agents per host. Container
name (`silkstrand-agent-<short-agent-id>`) and runtimes volume
(`silkstrand-agent-<short-agent-id>-runtimes`) follow the same
namespacing convention.

### D8. Service-manager parity

Binary mode uses `--as-service` to register with systemd/launchd.
Docker mode uses `--restart=unless-stopped` on `docker run`, which is
the idiomatic equivalent. No new systemd/launchd unit is written when
`--mode=docker`.

## Flow

1. Preflight: verify `docker` is on PATH; verify socket reachable
   (`docker info >/dev/null`); fail early with a helpful message
   otherwise. If `--docker-scan-all-bridges` set, enumerate bridges
   now and print the set that will be used.
2. Bootstrap: exchange install token for agent credentials by hitting
   the API directly (same endpoint the binary installer uses). Persist
   agent-id + agent-key into the named volume via a one-shot helper
   container so the main run is idempotent.
3. Render allowlist from flags, write to host path, `chmod 0444`.
4. `docker run -d --name silkstrand-agent --restart unless-stopped
   --network <first>  -e SILKSTRAND_AGENT_ID=... -e SILKSTRAND_AGENT_KEY=...
   -e SILKSTRAND_API_URL=... -e SILKSTRAND_NAABU_SCAN_TYPE=c
   -e SILKSTRAND_RUNTIMES_DIR=/home/nonroot/runtimes
   -v <volume>:/home/nonroot
   -v <allowlist-host-path>:/etc/silkstrand/scan-allowlist.yaml:ro
   <image>:<tag>`
5. For each additional network: `docker network connect <net>
   silkstrand-agent`.
6. Tail `docker logs` until we see `connected to server`, print a
   success banner, exit 0. On timeout, print a diagnostic banner
   pointing at `docker logs silkstrand-agent` and exit non-zero.

## What this ships in (PR split)

1. **PR 1 — installer**: `install.sh` gains `--mode=docker` and the
   flags above. Preflight, render, run, multi-network attach,
   success-tail. Shell only — no Go changes.
2. **PR 2 — docs**: README + onboarding page update showing the
   docker-mode one-liner alongside the binary one-liner. Explicit
   callout about naabu CONNECT mode + opt-in raw caps.
3. **PR 3 (optional)**: UI tweak in the Agents page install-command
   generator to surface a toggle / tab for "Docker host mode," which
   renders the corresponding `install.sh --mode=docker ...` one-liner
   with a pre-checked `--docker-scan-all-bridges` box.

## Resolved questions

- **Q1. `--docker-host-network` escape hatch — IN.** Linux-only flag,
  default off. Puts the agent on the host's network stack so one
  agent can reach both Docker bridges and host LAN without multi-homing.
  Installer errors helpfully on macOS (Docker Desktop doesn't support
  host networking).
- **Q2. Dual-agent path collision — handled by D7.** Per-agent-id
  host-side directory eliminates the collision; same naming
  convention extends to container name and runtimes volume so
  multiple docker agents can coexist per host (supports future
  horizontal scale-out).
- **Q3. Upgrade semantics — (a) + (c).** The server marks docker-mode
  agents as non-self-upgradeable. The Upgrade button renders a
  message pointing the user at `install.sh --mode=docker --upgrade`,
  which is the canonical path: installer pulls the newer image,
  stops the old container, and recreates it with the same flags
  (agent id + key persist via the named volume). No Docker socket
  access needed from the agent itself (preserves D6).
- **Q4. `--print-compose` subcommand — IN.** Emits a ready-to-save
  `docker-compose.yaml` snippet instead of running `docker run`.
  Mutually exclusive with the real run path; both share the flag
  surface and template.
