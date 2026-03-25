.PHONY: generate-api
generate-api:
	mkdir -p ./pkg/server
	mkdir -p ./pkg/amber
	go generate ./...

.PHONY: generate-mocks
generate-mocks:
	find mocks -mindepth 1 -name '*.go' -delete
	docker run --rm -v $(shell pwd):/src -w /src vektra/mockery:3

## generate-sqlc is intentionally removed: sqlc does not support SQL Server.
## The queries in internal/pkg/database/queries/ are now T-SQL (Azure SQL).
## The Azure Function DB layer will be written manually using database/sql.
## The existing generated code in internal/pkg/database/db/ remains committed
## until the local service DB dependency is removed in the wiring step.

.PHONY: buf-update
buf-update:
	docker run --rm -v $(shell pwd):/workspace --workdir /workspace bufbuild/buf:latest dep update

.PHONY: generate-proto
generate-proto:
	docker run --rm -v $(shell pwd):/workspace --workdir /workspace bufbuild/buf:latest generate

.PHONY: buf-update
buf-update:
	docker run --rm -v $(shell pwd):/workspace --workdir /workspace bufbuild/buf:latest dep update

.PHONY: generate-proto
generate-proto:
	docker run --rm -v $(shell pwd):/workspace --workdir /workspace bufbuild/buf:latest generate

.PHONY: buf-update
buf-update:
	docker run --rm -v $(shell pwd):/workspace --workdir /workspace bufbuild/buf:latest dep update

.PHONY: generate-proto
generate-proto:
	docker run --rm -v $(shell pwd):/workspace --workdir /workspace bufbuild/buf:latest generate

.PHONY: gen-all
gen-all: generate-api generate-mocks generate-sqlc generate-proto

.PHONY: test
test:
	go test -cover ./... -count=1

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: build
build:
	go build .

.PHONY: lint
lint:
	golangci-lint run ./...
