# Single-process agent image. Supports the install-token bootstrap flow so
# running `docker run ... -e SILKSTRAND_INSTALL_TOKEN=...` is enough to
# register and start scanning. Credentials are persisted to /var/lib/silkstrand
# (mount a volume to survive restarts).

FROM golang:1.24-alpine AS builder
ARG VERSION=dev
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/silkstrand-agent \
    ./cmd/silkstrand-agent/

# Distroless for a tiny, no-shell runtime. The agent runs Python assessment
# bundles, but those are fetched at scan time — not baked in.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/silkstrand-agent /usr/local/bin/silkstrand-agent
# nonroot user's home dir is /home/nonroot. Use it for the creds store when
# /var/lib/silkstrand isn't writable (e.g. read-only root FS).
ENV SILKSTRAND_CREDS_PATH=/home/nonroot/agent.creds
USER nonroot
ENTRYPOINT ["/usr/local/bin/silkstrand-agent"]
