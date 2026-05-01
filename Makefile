.PHONY: test test-cover lint build proto-gen

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
		specs/012-multiserver-pro/contracts/ingest.proto