.PHONY: build run

build:
	go build ./...

run: build
	go run ./cmd/rfs