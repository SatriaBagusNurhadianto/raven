# Raven build targets
.PHONY: build test vet fmt check migrate-up migrate-down run docker-build compose-up compose-down clean

build:
	go build -o bin/raven ./cmd/raven

test:
	go test ./... -race -count=1

test-integration:
	RAVEN_TEST_DATABASE_URL ?= postgres://raven:raven@localhost:5432/raven?sslmode=disable
	RAVEN_TEST_DATABASE_URL=$(RAVEN_TEST_DATABASE_URL) go test ./internal/db/ -tags=integration -v

vet:
	go vet ./...

fmt:
	gofmt -w .

check: fmt vet test

migrate-up:
	go run ./cmd/raven migrate up

migrate-down:
	go run ./cmd/raven migrate down

migrate-status:
	go run ./cmd/raven migrate status

run:
	go run ./cmd/raven serve

doctor:
	go run ./cmd/raven doctor

docker-build:
	docker build -f deploy/Dockerfile -t raven:dev .

compose-up:
	docker compose -f deploy/docker-compose.yml up -d

compose-down:
	docker compose -f deploy/docker-compose.yml down

clean:
	rm -rf bin/ data/
