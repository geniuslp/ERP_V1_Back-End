.PHONY: run build docker-up docker-down swagger tidy migrate seed

# ── Local dev ──────────────────────────────────────────────────────────────────
run:
	go run ./cmd/server/main.go

build:
	go build -o bin/erp-api ./cmd/server

# ── Swagger ────────────────────────────────────────────────────────────────────
swagger:
	swag init -g cmd/server/main.go -o docs --parseDependency --parseInternal
	@echo "✅  Swagger docs generated → docs/"

install-swag:
	go install github.com/swaggo/swag/cmd/swag@latest

# ── Docker ────────────────────────────────────────────────────────────────────
docker-up:
	docker compose up --build -d
	@echo "🚀  API:     http://localhost:8080"
	@echo "📚  Swagger: http://localhost:8080/swagger/index.html"

docker-down:
	docker compose down

docker-tools:
	docker compose --profile tools up -d

docker-logs:
	docker compose logs -f api

# ── Database ──────────────────────────────────────────────────────────────────
migrate:
	psql $${DATABASE_URL} -f migrations/001_master_ddl.sql
	@echo "✅  Migration applied"

# ── Deps ──────────────────────────────────────────────────────────────────────
tidy:
	go mod tidy

test:
	go test ./... -v -cover
