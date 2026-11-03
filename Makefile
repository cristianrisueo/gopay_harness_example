MIGRATIONS_PATH = ./migrate/migrations
MIGRATIONS_DB_ADDRESS = postgres://admin:password@localhost/gopay?sslmode=disable
DB_CONTAINER = postgres-db
DB_USER = admin
DB_NAME = gopay


# ==================== Docker Commands ====================

.PHONY: docker-build
docker-build:
	@echo "Building Docker image..."
	@docker compose build app
	@echo "✓ Docker image built successfully!"

.PHONY: docker-up
docker-up:
	@echo "Starting all services..."
	@docker compose up -d
	@echo "✓ All services started!"
	@echo ""
	@echo "API running at: http://localhost:${PORT:-8080}"
	@echo "PostgreSQL running at: localhost:${DB_PORT:-5432}"

.PHONY: docker-down
docker-down:
	@echo "Stopping all services..."
	@docker compose down
	@echo "✓ All services stopped!"

# ==================== Database (docker) Commands ====================

.PHONY: migrate-up
migrate-up:
	@migrate -path=$(MIGRATIONS_PATH) -database=$(MIGRATIONS_DB_ADDRESS) up

.PHONY: migrate-down
migrate-down:
	@migrate -path=$(MIGRATIONS_PATH) -database=$(MIGRATIONS_DB_ADDRESS) down

.PHONY: seed
seed:
	@echo "Seeding database..."
	@docker exec -i $(DB_CONTAINER) psql -U $(DB_USER) -d $(DB_NAME) < migrate/seed/seed.sql
	@echo "✓ Database seeded successfully!"

.PHONY: reset-db
reset-db:
	@echo "Resetting database..."
	@docker exec $(DB_CONTAINER) psql -U $(DB_USER) -d postgres -c "DROP DATABASE IF EXISTS $(DB_NAME);"
	@docker exec $(DB_CONTAINER) psql -U $(DB_USER) -d postgres -c "CREATE DATABASE $(DB_NAME);"
	@echo "✓ Database dropped and recreated"
	@echo "Running migrations..."
	@make migrate-up
	@echo "Running seed..."
	@make seed
	@echo "✓ Database reset complete!"

# ==================== Test Commands ====================

.PHONY: check
check: harness-test
	@echo "Checking formatting and running go vet..."
	@unformatted="$$(gofmt -l cmd internal pkg)"; \
	if [ -n "$$unformatted" ]; then echo "gofmt -l lists unformatted files:"; echo "$$unformatted"; exit 1; fi
	@go vet ./...
	@echo "✓ Checks passed!"

.PHONY: test
test:
	@echo "Running unit tests..."
	@go test -race -count=1 ./...

.PHONY: test-integration
test-integration:
	@echo "Running integration tests (requires Docker)..."
	@go test -race -count=1 -tags integration ./...

# ==================== Harness Commands ====================

.PHONY: harness-test
harness-test:
	@echo "Running harness tests..."
	@bash .claude/hooks/guard_test.sh
