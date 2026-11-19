.PHONY: test run fmt

test:
	go test -race ./...

run:
	go run ./cmd/server

fmt:
	go fmt ./...