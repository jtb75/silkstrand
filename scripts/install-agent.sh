#!/usr/bin/env sh
# SilkStrand agent installer.
#
# Usage:
#   curl -sSL https://storage.googleapis.com/silkstrand-agent-releases/install.sh | sh
#
# Installs silkstrand-agent to $INSTALL_DIR (default /usr/local/bin) for the
# detected OS and architecture, verifying the SHA-256 checksum against the
# file published alongside the binary.
#
# Env:
#   INSTALL_DIR   Where to install the binary (default /usr/local/bin)
#   VERSION       Release tag to pull (default "latest")
#   RELEASE_URL   Override the GCS base URL (for mirrors / dev)

set -eu

INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
VERSION="${VERSION:-latest}"
RELEASE_URL="${RELEASE_URL:-https://storage.googleapis.com/silkstrand-agent-releases}"

detect_os() {
    os=$(uname -s | tr '[:upper:]' '[:lower:]')
    case "$os" in
        linux|darwin) echo "$os" ;;
        *) echo "unsupported OS: $os" >&2; exit 1 ;;
    esac
}

detect_arch() {
    arch=$(uname -m)
    case "$arch" in
        x86_64|amd64) echo "amd64" ;;
        arm64|aarch64) echo "arm64" ;;
        *) echo "unsupported arch: $arch" >&2; exit 1 ;;
    esac
}

main() {
    os=$(detect_os)
    arch=$(detect_arch)
    suffix="${os}-${arch}"
    bin_url="${RELEASE_URL}/${VERSION}/silkstrand-agent-${suffix}"
    sha_url="${bin_url}.sha256"
    tmp=$(mktemp -d)
    trap 'rm -rf "$tmp"' EXIT

    echo "Downloading silkstrand-agent (${suffix}, ${VERSION})…"
    curl -fsSL -o "$tmp/silkstrand-agent" "$bin_url"
    curl -fsSL -o "$tmp/silkstrand-agent.sha256" "$sha_url"

    echo "Verifying checksum…"
    expected=$(cut -d' ' -f1 "$tmp/silkstrand-agent.sha256")
    actual=$(shasum -a 256 "$tmp/silkstrand-agent" | cut -d' ' -f1)
    if [ "$expected" != "$actual" ]; then
        echo "checksum mismatch: expected $expected, got $actual" >&2
        exit 1
    fi

    chmod +x "$tmp/silkstrand-agent"

    if [ -w "$INSTALL_DIR" ]; then
        mv "$tmp/silkstrand-agent" "$INSTALL_DIR/silkstrand-agent"
    else
        echo "Installing to $INSTALL_DIR (requires sudo)"
        sudo mv "$tmp/silkstrand-agent" "$INSTALL_DIR/silkstrand-agent"
    fi

    echo ""
    echo "Installed: $INSTALL_DIR/silkstrand-agent"
    echo ""
    echo "Next step: run with your agent credentials."
    echo ""
    echo "  SILKSTRAND_AGENT_ID=<uuid> \\"
    echo "  SILKSTRAND_AGENT_KEY=<key> \\"
    echo "  SILKSTRAND_API_URL=wss://<your DC API host> \\"
    echo "  silkstrand-agent"
    echo ""
    echo "Credentials are created from the SilkStrand tenant UI under Agents."
}

main "$@"
