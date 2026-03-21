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

.PHONY: test
test:
	go test -cover ./... -count=1

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: build
build:
	go build .

