.PHONY: test lint dev dev-fake db-up db-down dev-stack-up dev-stack-down dev-stack-check db-migrate db-sync-fake db-test fixtures-check migrations-check provider-contract-test web-test web-lint

export GOCACHE := $(CURDIR)/.cache/go-build
export GOPATH := $(CURDIR)/.cache/go
export GOMODCACHE := $(GOPATH)/pkg/mod

test:
	go test ./...

lint:
	@gofmt_out="$$(gofmt -l $$(find . -name '*.go' -not -path './.git/*' -not -path './.cache/*'))"; \
	if [ -n "$$gofmt_out" ]; then \
		echo "gofmt required:"; \
		echo "$$gofmt_out"; \
		exit 1; \
	fi
	go vet ./...
	go test ./internal/api -run TestOpenAPISpecMatchesRegisteredRoutes -count=1
	sh scripts/check_migrations.sh
	go test ./...

dev:
	ATLAS_PROVIDER_MODE=$${ATLAS_PROVIDER_MODE:-fake} go run ./cmd/atlas-api

dev-fake:
	ATLAS_PROVIDER_MODE=fake go run ./cmd/atlas-api

db-up:
	docker compose -f dev/docker-compose.yml up -d postgres

db-down:
	docker compose -f dev/docker-compose.yml down

dev-stack-up:
	docker compose -f dev/docker-compose.yml --profile stack up --build

dev-stack-down:
	docker compose -f dev/docker-compose.yml --profile stack down

dev-stack-check:
	sh scripts/dev_stack_check.sh

db-migrate:
	sh scripts/apply_migrations.sh

db-sync-fake: db-migrate
	ATLAS_PROVIDER_MODE=fake go run ./cmd/atlas-inventory-sync

db-test:
	@set -e; \
	database_url="$${ATLAS_TEST_DATABASE_URL:-postgres://atlas:atlas_dev@127.0.0.1:15432/atlas_test?sslmode=disable}"; \
	admin_url="$${ATLAS_TEST_ADMIN_DATABASE_URL:-postgres://atlas:atlas_dev@127.0.0.1:15432/postgres?sslmode=disable}"; \
	if [ -z "$${ATLAS_TEST_DATABASE_URL:-}" ]; then \
		psql "$$admin_url" -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS atlas_test" >/dev/null; \
		psql "$$admin_url" -v ON_ERROR_STOP=1 -c "CREATE DATABASE atlas_test" >/dev/null; \
	fi; \
	DATABASE_URL="$$database_url" sh scripts/apply_migrations.sh; \
	ATLAS_TEST_DATABASE_URL="$$database_url" go test ./internal/store ./internal/inventorysync -count=1; \
	ATLAS_TEST_DATABASE_URL="$$database_url" go test ./internal/api -count=1

fixtures-check:
	go test ./internal/providers/fake -run TestFixtures

provider-contract-test:
	go test ./internal/providers/... -run TestCephReadProviderContract -count=1

migrations-check:
	sh scripts/check_migrations.sh

web-test:
	cd web/app && npm test

web-lint:
	cd web/app && npm run lint
