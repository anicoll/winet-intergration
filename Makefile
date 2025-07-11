generate-api:
	mkdir -p ./pkg/server
	mkdir -p ./pkg/amber
	go generate ./...

.PHONY: test
test:
	go test -cover ./... -count=1

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: build
build:
	go build .

gen-models:
	mkdir -p ./internal/pkg/models
	/root/go/bin/dbtpl schema ${DATABASE_URL} --out ./internal/pkg/models
