.PHONY: test test-cover lint build proto-gen test-pg

test:
	go test ./...

test-cover:
	go test -coverprofile=cover.out ./... && go tool cover -html=cover.out

lint:
	golangci-lint run

build:
	go build -o ./bin/maintenant ./cmd/maintenant

proto-gen:
	mkdir -p internal/agentpb
	protoc \
		--go_out=. \
		--go_opt=module=github.com/kolapsis/maintenant \
		--go-grpc_out=. \
		--go-grpc_opt=module=github.com/kolapsis/maintenant \
		--proto_path=. \
		proto/ingest.proto
# Disposable PostgreSQL for the store test suite. The suite skips PostgreSQL
# cases when MAINTENANT_TEST_DATABASE_URL is unset, so this stays opt-in.
test-pg:
	docker rm -f maintenant-test-pg 2>/dev/null || true
	docker run --rm -d --name maintenant-test-pg -e POSTGRES_PASSWORD=test -p 127.0.0.1:54329:5432 postgres:14-alpine
	until docker exec maintenant-test-pg pg_isready -U postgres -q; do sleep 0.5; done
	MAINTENANT_TEST_DATABASE_URL="postgres://postgres:test@127.0.0.1:54329/postgres?sslmode=disable" go test -race ./internal/store/...
	docker rm -f maintenant-test-pg
