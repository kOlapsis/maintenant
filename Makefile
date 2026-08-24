.PHONY: test test-cover lint build proto-gen test-pg \
	e2e-sqlite e2e-postgres e2e-both e2e-up-sqlite e2e-up-postgres e2e-logs e2e-down e2e-migrate

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

# ---------------------------------------------------------------------------
# End-to-end: the whole product, on either engine.
#
# The same stack and the same checks run twice, once per engine, so anything
# that behaves differently on PostgreSQL shows up here rather than in
# production. SQLite needs no extra service; PostgreSQL is an overlay.
# ---------------------------------------------------------------------------

E2E_BASE    := -f compose.test.yml
E2E_PG      := -f compose.test.yml -f compose.test.postgres.yml

e2e-sqlite:
	docker compose $(E2E_BASE) up -d --build
	scripts/e2e-check.sh sqlite

e2e-postgres:
	docker compose $(E2E_PG) up -d --build
	scripts/e2e-check.sh postgres

## Both engines in turn, each on a clean stack. This is the one that proves
## the product does the same thing whichever database backs it.
e2e-both:
	$(MAKE) e2e-down
	$(MAKE) e2e-sqlite
	$(MAKE) e2e-down
	$(MAKE) e2e-postgres
	$(MAKE) e2e-down

## Leave the stack running to poke at it: http://127.0.0.1:18090
e2e-up-sqlite:
	docker compose $(E2E_BASE) up -d --build

e2e-up-postgres:
	docker compose $(E2E_PG) up -d --build

e2e-logs:
	docker compose $(E2E_PG) logs -f maintenant

## Tear everything down, volumes included: the next run starts from an empty
## database, which is what makes the two engines comparable.
e2e-down:
	docker compose $(E2E_PG) down -v --remove-orphans 2>/dev/null || true
	docker compose $(E2E_BASE) down -v --remove-orphans 2>/dev/null || true

## Migrate a running SQLite test stack onto the PostgreSQL one, the way an
## operator would: stop, copy, restart on the database. The copy is the
## copy-store service of the overlay, which is where the connection string
## lives; run starts the database and waits for it to be healthy.
e2e-migrate:
	docker compose $(E2E_BASE) stop maintenant
	docker compose $(E2E_PG) run --rm copy-store
	docker compose $(E2E_PG) up -d maintenant
	scripts/e2e-check.sh postgres
