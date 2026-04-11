# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN GOTOOLCHAIN=auto go mod download

COPY . .

RUN GOTOOLCHAIN=auto CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /silkstrand-api \
    ./cmd/silkstrand-api/

# Run stage
FROM gcr.io/distroless/static-debian12

COPY --from=builder /silkstrand-api /silkstrand-api

EXPOSE 8080

ENTRYPOINT ["/silkstrand-api"]
