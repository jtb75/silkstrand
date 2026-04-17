# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy go module files first for caching.
COPY api/go.mod api/go.sum ./
RUN GOTOOLCHAIN=auto go mod download

# Copy API source.
COPY api/ .

RUN GOTOOLCHAIN=auto CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /silkstrand-api \
    ./cmd/silkstrand-api/

# Run stage
FROM gcr.io/distroless/static-debian12

COPY --from=builder /silkstrand-api /silkstrand-api

# Individual control directories for server-side bundle assembly (Level 3B).
COPY controls/ /app/controls/

ENV BUNDLE_CONTROLS_DIR=/app/controls

EXPOSE 8080

ENTRYPOINT ["/silkstrand-api"]
