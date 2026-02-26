.PHONY: build run test clean

build:
	go build -o bin/labelr ./cmd/labelr

run:
	go run ./cmd/labelr

test:
	go test ./... -v

clean:
	rm -rf bin/
