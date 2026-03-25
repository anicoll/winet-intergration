.PHONY: generate-api
generate-api:
	mkdir -p ./pkg/server
	mkdir -p ./pkg/amber
	go generate ./...

.PHONY: generate-mocks
generate-mocks:
	find mocks -mindepth 1 -name '*.go' -delete
	docker run --rm -v $(shell pwd):/src -w /src vektra/mockery:3

.PHONY: generate-sqlc
generate-sqlc:
	docker run --rm -v $(shell pwd):/src -w /src sqlc/sqlc:1.28.0 generate

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
