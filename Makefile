.PHONY: dev dev-deps build test lint docker run migrate-up migrate-down clean seed seed-mssql seed-mongo jwt hash-password

# Start local dependencies (Postgres + Redis)
dev-deps:
	docker compose up -d

# Run the API server locally (requires dev-deps)
# Loads api/.env.local if present for Clerk and other config
run:
	cd api && if [ -f .env.local ]; then set -a && . ./.env.local && set +a; fi && go run ./cmd/silkstrand-api/

# Start everything for local development
dev: dev-deps
	@echo "Waiting for Postgres..."
	@until docker compose exec -T postgres pg_isready -U silkstrand > /dev/null 2>&1; do sleep 1; done
	@echo "Dependencies ready. Starting API..."
	cd api && if [ -f .env.local ]; then set -a && . ./.env.local && set +a; fi && go run ./cmd/silkstrand-api/

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

# --- Seed & Dev Helpers ---

# Seed local databases with test data (requires docker compose up)
seed:
	bash scripts/seed.sh

# Seed the local SQL Server 2022 container with a read-only scan user.
seed-mssql:
	bash scripts/seed-mssql.sh

# Seed the local MongoDB 8 container with a read-only scan user.
seed-mongo:
	bash scripts/seed-mongo.sh

# Generate a dev JWT token for the default test tenant
jwt:
	python3 scripts/gen-jwt.py

# Generate a bcrypt hash (usage: make hash-password PASS=mysecret)
hash-password:
	cd backoffice && go run ../scripts/hash-password.go $(or $(PASS),admin123)

# --- Infrastructure ---

# Stop local dependencies
down:
	docker compose down

# Clean up
clean:
	docker compose down -v
	rm -rf bin/
