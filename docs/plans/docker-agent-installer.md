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

### D7. Allowlist lives in a host file, bind-mounted RO

Installer writes `/etc/silkstrand/docker-agent-<agent-id>/scan-allowlist.yaml`
(or Docker-for-Mac equivalent under `~/.silkstrand/…`) and bind-mounts
it into the container at `/etc/silkstrand/scan-allowlist.yaml:ro`.
Rationale: lets the user edit the file in place and restart the
container to refresh, just like the binary-mode workflow. Avoids
baking the allowlist into the image or a named volume.

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

## Open questions

- **Q1. Do we need a `--docker-host-network` escape hatch?** Linux-only.
  Would put the agent on the host's network stack so it can reach both
  Docker bridges *and* the host LAN from a single agent, no
  multi-homing. Likely yes as a flag, default off.
- **Q2. Allowlist UX for two agents on one host.** If the user runs
  both binary-mode (scanning LAN) and docker-mode (scanning bridges)
  on the same host, do we collide on the `/etc/silkstrand` path? The
  per-agent-id directory in D7 handles this on the docker side, but we
  should document the pattern rather than rely on luck.
- **Q3. How should upgrades work?** Binary mode's `Upgrade` button
  tells the agent to self-replace its binary and exit. Docker mode
  can't do that (image is immutable). Options: (a) Upgrade button
  sends a signal that causes `docker restart` via a host-side helper
  (needs socket access — violates D6), (b) Upgrade becomes a no-op for
  docker agents and the installer re-run path is the upgrade path,
  (c) a separate `upgrade` sidecar container that uses
  `SILKSTRAND_INSTALL_TOKEN`-style re-bootstrap. Leaning (b) —
  simplest, matches how most containerized services upgrade — but
  needs a UI message so the button doesn't mislead.
- **Q4. Should we also publish a `docker-compose.yaml` snippet?** Some
  users will want to manage the agent alongside their other services.
  Cheap to generate, worth including as an installer subcommand
  (`install.sh --mode=docker --print-compose`).
