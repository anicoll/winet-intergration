generate-api:
	mkdir -p ./pkg/api
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
