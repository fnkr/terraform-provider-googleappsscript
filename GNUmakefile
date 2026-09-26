default: build

build:
	go build ./...

lint:
	golangci-lint run

generate:
	go generate ./...

test:
	go test ./...

testacc:
	TF_ACC=1 go test ./... -count=1 -v -timeout 30m

.PHONY: default build lint generate test testacc
