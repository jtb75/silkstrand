#!/usr/bin/env sh
# SilkStrand agent installer.
#
# One-shot flow:
#   curl -sSL https://storage.googleapis.com/silkstrand-agent-releases/install.sh | \
#     sudo sh -s -- \
#       --token=sst_<install-token> \
#       --api-url=https://<your DC API host> \
#       --name=$(hostname) \
#       --as-service
#
# What happens:
#   1. Download + verify the silkstrand-agent binary for this OS/arch.
#   2. Install to $INSTALL_DIR (default /usr/local/bin).
#   3. Exchange the install token for long-lived agent credentials.
#   4. Write /etc/silkstrand/agent.env (mode 0600, root-owned).
#   5. If --as-service: install a systemd unit (Linux) or launchd plist
#      (macOS) and start the agent.
#
# Flags:
#   --token=TOK            One-time install token from the SilkStrand UI
#   --api-url=URL          Your DC's HTTPS URL (same host, wss:// is derived)
#   --name=NAME            Friendly name for this agent (default: hostname)
#   --as-service           Install + start a system service
#   --no-service           Skip service install (default)
#   --uninstall            Remove the agent: notify server, stop service,
#                          delete binary + /etc/silkstrand
#   --install-dir=PATH     Where to install the binary (default /usr/local/bin)
#   --version=TAG          Release to download (default "latest")
#   --release-url=URL      Override the GCS base for binaries (dev / mirrors)

set -eu

INSTALL_DIR="/usr/local/bin"
VERSION="latest"
RELEASE_URL="https://storage.googleapis.com/silkstrand-agent-releases"
TOKEN=""
API_URL=""
NAME="$(uname -n 2>/dev/null || echo agent)"
AS_SERVICE=0
UNINSTALL=0
CONFIG_DIR="/etc/silkstrand"
CONFIG_FILE="/etc/silkstrand/agent.env"

log() { printf '==> %s\n' "$*"; }
fail() { printf 'error: %s\n' "$*" >&2; exit 1; }

parse_args() {
    while [ $# -gt 0 ]; do
        case "$1" in
            --token=*)       TOKEN="${1#*=}" ;;
            --api-url=*)     API_URL="${1#*=}" ;;
            --name=*)        NAME="${1#*=}" ;;
            --install-dir=*) INSTALL_DIR="${1#*=}" ;;
            --version=*)     VERSION="${1#*=}" ;;
            --release-url=*) RELEASE_URL="${1#*=}" ;;
            --as-service)    AS_SERVICE=1 ;;
            --no-service)    AS_SERVICE=0 ;;
            --uninstall)     UNINSTALL=1 ;;
            -h|--help)       grep '^#' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
            *) fail "unknown flag: $1" ;;
        esac
        shift
    done
}

detect_os() {
    os=$(uname -s | tr '[:upper:]' '[:lower:]')
    case "$os" in
        linux|darwin) printf '%s' "$os" ;;
        *) fail "unsupported OS: $os" ;;
    esac
}

detect_arch() {
    arch=$(uname -m)
    case "$arch" in
        x86_64|amd64) printf 'amd64' ;;
        arm64|aarch64) printf 'arm64' ;;
        *) fail "unsupported arch: $arch" ;;
    esac
}

need() { command -v "$1" >/dev/null 2>&1 || fail "'$1' is required"; }

need_root() {
    if [ "$(id -u)" -ne 0 ]; then
        fail "run this script with sudo (writes to $INSTALL_DIR and $CONFIG_DIR)"
    fi
}

download_binary() {
    suffix="$(detect_os)-$(detect_arch)"
    bin_url="${RELEASE_URL}/${VERSION}/silkstrand-agent-${suffix}"
    sha_url="${bin_url}.sha256"
    tmp=$(mktemp -d)
    trap 'rm -rf "$tmp"' EXIT

    log "Downloading silkstrand-agent (${suffix}, ${VERSION})"
    curl -fsSL -o "$tmp/silkstrand-agent" "$bin_url"
    curl -fsSL -o "$tmp/silkstrand-agent.sha256" "$sha_url"

    log "Verifying checksum"
    expected=$(cut -d' ' -f1 "$tmp/silkstrand-agent.sha256")
    if command -v sha256sum >/dev/null 2>&1; then
        actual=$(sha256sum "$tmp/silkstrand-agent" | cut -d' ' -f1)
    else
        actual=$(shasum -a 256 "$tmp/silkstrand-agent" | cut -d' ' -f1)
    fi
    if [ "$expected" != "$actual" ]; then
        fail "checksum mismatch: expected $expected, got $actual"
    fi

    chmod +x "$tmp/silkstrand-agent"
    install -d "$INSTALL_DIR"
    mv "$tmp/silkstrand-agent" "$INSTALL_DIR/silkstrand-agent"
    log "Installed $INSTALL_DIR/silkstrand-agent"
}

bootstrap_agent() {
    [ -n "$TOKEN" ] || fail "--token is required"
    [ -n "$API_URL" ] || fail "--api-url is required"

    # POST bootstrap with the install token; server returns agent_id + api_key.
    agent_version=$("$INSTALL_DIR/silkstrand-agent" version 2>/dev/null || echo "")
    log "Registering agent '$NAME' (version $agent_version)"
    payload=$(printf '{"install_token":"%s","name":"%s","version":"%s"}' "$TOKEN" "$NAME" "$agent_version")
    resp=$(curl -fsS -X POST \
        -H 'Content-Type: application/json' \
        -d "$payload" \
        "${API_URL}/api/v1/agents/bootstrap")

    # Minimal JSON parse without jq: extract agent_id and api_key by key.
    agent_id=$(printf '%s' "$resp" | sed -n 's/.*"agent_id"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
    api_key=$(printf '%s' "$resp" | sed -n 's/.*"api_key"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
    [ -n "$agent_id" ] || fail "server did not return agent_id (response: $resp)"
    [ -n "$api_key" ]  || fail "server did not return api_key"

    # DC WSS URL is the API URL with scheme swapped. Keep the host.
    ws_url=$(printf '%s' "$API_URL" | sed -e 's,^https://,wss://,' -e 's,^http://,ws://,')

    install -d -m 0700 "$CONFIG_DIR"
    umask 077
    cat > "$CONFIG_FILE" <<EOF
# SilkStrand agent — written by install.sh.
# mode 0600 — do not share.
SILKSTRAND_AGENT_ID=$agent_id
SILKSTRAND_AGENT_KEY=$api_key
SILKSTRAND_API_URL=$ws_url
EOF
    chmod 0600 "$CONFIG_FILE"
    log "Credentials written to $CONFIG_FILE"
    log "Agent ID: $agent_id"
}

install_service_linux() {
    unit=/etc/systemd/system/silkstrand-agent.service
    cat > "$unit" <<EOF
[Unit]
Description=SilkStrand compliance scanner agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=$CONFIG_FILE
ExecStart=$INSTALL_DIR/silkstrand-agent
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
    chmod 0644 "$unit"
    systemctl daemon-reload
    systemctl enable --now silkstrand-agent
    log "silkstrand-agent service started (systemd)"
    log "Tail logs: journalctl -u silkstrand-agent -f"
}

install_service_darwin() {
    plist=/Library/LaunchDaemons/io.silkstrand.agent.plist
    # launchd can't read an EnvironmentFile, so load the env vars here.
    # We read the env file at install time and bake into the plist.
    set -a; . "$CONFIG_FILE"; set +a
    cat > "$plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>io.silkstrand.agent</string>
  <key>ProgramArguments</key>
    <array><string>$INSTALL_DIR/silkstrand-agent</string></array>
  <key>EnvironmentVariables</key><dict>
    <key>SILKSTRAND_AGENT_ID</key><string>${SILKSTRAND_AGENT_ID}</string>
    <key>SILKSTRAND_AGENT_KEY</key><string>${SILKSTRAND_AGENT_KEY}</string>
    <key>SILKSTRAND_API_URL</key><string>${SILKSTRAND_API_URL}</string>
  </dict>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>/var/log/silkstrand-agent.log</string>
  <key>StandardErrorPath</key><string>/var/log/silkstrand-agent.log</string>
</dict></plist>
EOF
    chmod 0644 "$plist"
    launchctl bootout system/io.silkstrand.agent 2>/dev/null || true
    launchctl bootstrap system "$plist"
    log "silkstrand-agent service started (launchd)"
    log "Tail logs: tail -f /var/log/silkstrand-agent.log"
}

install_service() {
    case "$(detect_os)" in
        linux)  install_service_linux ;;
        darwin) install_service_darwin ;;
    esac
}

print_manual_run() {
    cat <<EOF

Next step (manual run — no service installed):

  sudo sh -c '. $CONFIG_FILE && exec $INSTALL_DIR/silkstrand-agent'

Or re-run install.sh with --as-service to install a system service.
EOF
}

uninstall_service_linux() {
    if [ -f /etc/systemd/system/silkstrand-agent.service ]; then
        systemctl disable --now silkstrand-agent 2>/dev/null || true
        rm -f /etc/systemd/system/silkstrand-agent.service
        systemctl daemon-reload
        log "removed systemd unit"
    fi
}

uninstall_service_darwin() {
    if [ -f /Library/LaunchDaemons/io.silkstrand.agent.plist ]; then
        launchctl bootout system/io.silkstrand.agent 2>/dev/null || true
        rm -f /Library/LaunchDaemons/io.silkstrand.agent.plist
        log "removed launchd plist"
    fi
}

uninstall_self_delete() {
    # Best-effort call to /api/v1/agents/self so the tenant UI doesn't show
    # a ghost entry. Ignores failures (network, already-deleted, etc.).
    if [ ! -f "$CONFIG_FILE" ]; then return 0; fi
    # shellcheck disable=SC1090
    . "$CONFIG_FILE" || return 0
    [ -n "${SILKSTRAND_AGENT_ID:-}" ] || return 0
    [ -n "${SILKSTRAND_AGENT_KEY:-}" ] || return 0
    [ -n "${SILKSTRAND_API_URL:-}" ] || return 0

    # Swap wss:// → https:// for the HTTP call.
    http_url=$(printf '%s' "$SILKSTRAND_API_URL" | sed -e 's,^wss://,https://,' -e 's,^ws://,http://,')
    log "Notifying server: agent ${SILKSTRAND_AGENT_ID}"
    curl -fsS -X DELETE \
        -H "Authorization: Bearer $SILKSTRAND_AGENT_KEY" \
        "${http_url}/api/v1/agents/self?agent_id=${SILKSTRAND_AGENT_ID}" \
        >/dev/null 2>&1 || log "server notify failed (continuing with local cleanup)"
}

do_uninstall() {
    need_root
    uninstall_self_delete
    case "$(detect_os)" in
        linux)  uninstall_service_linux ;;
        darwin) uninstall_service_darwin ;;
    esac
    rm -f "$INSTALL_DIR/silkstrand-agent"
    rm -rf "$CONFIG_DIR"
    log "Uninstalled silkstrand-agent"
}

main() {
    parse_args "$@"
    need curl
    if [ "$UNINSTALL" -eq 1 ]; then
        do_uninstall
        return 0
    fi
    need_root
    download_binary
    if [ -n "$TOKEN" ] || [ -n "$API_URL" ]; then
        bootstrap_agent
        if [ "$AS_SERVICE" -eq 1 ]; then
            install_service
        else
            print_manual_run
        fi
    else
        cat <<EOF

Binary installed. You still need credentials to run the agent.
Generate an install token in the SilkStrand UI and re-run:

  curl -sSL ${RELEASE_URL}/install.sh | sudo sh -s -- \\
    --token=<token> --api-url=<DC url> --name=\$(hostname) --as-service
EOF
    fi
}

main "$@"
