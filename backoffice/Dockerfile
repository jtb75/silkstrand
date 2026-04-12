# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN GOTOOLCHAIN=auto go mod download

COPY . .

RUN GOTOOLCHAIN=auto CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /backoffice-api \
    ./cmd/backoffice-api/

# Run stage
FROM gcr.io/distroless/static-debian12

COPY --from=builder /backoffice-api /backoffice-api

EXPOSE 8081

ENTRYPOINT ["/backoffice-api"]
