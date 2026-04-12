.PHONY: dev dev-deps build test lint docker run migrate-up migrate-down clean

# Start local dependencies (Postgres + Redis)
dev-deps:
	docker compose up -d

# Run the API server locally (requires dev-deps)
run:
	cd api && go run ./cmd/silkstrand-api/

# Start everything for local development
dev: dev-deps
	@echo "Waiting for Postgres..."
	@until docker compose exec -T postgres pg_isready -U silkstrand > /dev/null 2>&1; do sleep 1; done
	@echo "Dependencies ready. Starting API..."
	cd api && go run ./cmd/silkstrand-api/

# Build the API binary
build:
	cd api && CGO_ENABLED=0 go build -ldflags="-s -w" -o ../bin/silkstrand-api ./cmd/silkstrand-api/

# Run tests
test:
	cd api && go test ./... -v -race

# Run linter
lint:
	cd api && golangci-lint run ./...

# Build Docker image
docker:
	docker build -t silkstrand-api:local -f api/Dockerfile api/

# --- Backoffice ---

run-backoffice:
	cd backoffice && go run ./cmd/backoffice-api/

build-backoffice:
	cd backoffice && CGO_ENABLED=0 go build -ldflags="-s -w" -o ../bin/backoffice-api ./cmd/backoffice-api/

test-backoffice:
	cd backoffice && go test ./... -v -race

lint-backoffice:
	cd backoffice && golangci-lint run ./...

# --- Infrastructure ---

# Stop local dependencies
down:
	docker compose down

# Clean up
clean:
	docker compose down -v
	rm -rf bin/
