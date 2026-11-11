.PHONY: up down migrate api web test test-integration lint fmt

DATABASE_URL ?= postgres://aaa@localhost:5432/cocok?sslmode=disable
TEST_DATABASE_URL ?= postgres://aaa@localhost:5432/cocok_test?sslmode=disable

up:
	docker compose up -d

down:
	docker compose down

migrate:
	cd services/api && DATABASE_URL=$(DATABASE_URL) go run ./cmd/server migrate

api:
	cd services/api && DATABASE_URL=$(DATABASE_URL) SERVER_ADDR=:8081 WEB_ORIGIN=http://localhost:3000 go run ./cmd/server

web:
	cd apps/web && NEXT_PUBLIC_API_URL=http://localhost:8081 npm run dev

test:
	cd services/api && go vet ./... && go test ./...

test-integration:
	cd services/api && TEST_DATABASE_URL=$(TEST_DATABASE_URL) go test ./...

sqlc:
	cd services/api && sqlc generate

lint:
	cd apps/web && npm run typecheck

fmt:
	cd services/api && gofmt -w ./cmd ./internal ./tests
