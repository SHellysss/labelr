.PHONY: build run test clean release

build:
	go build -o bin/labelr ./cmd/labelr

run:
	go run ./cmd/labelr

test:
	go test ./... -v

clean:
	rm -rf bin/

release:
	GOOS=darwin GOARCH=arm64 go build -o bin/labelr-darwin-arm64 ./cmd/labelr
	GOOS=darwin GOARCH=amd64 go build -o bin/labelr-darwin-amd64 ./cmd/labelr
	GOOS=linux GOARCH=amd64 go build -o bin/labelr-linux-amd64 ./cmd/labelr
	GOOS=windows GOARCH=amd64 go build -o bin/labelr-windows-amd64.exe ./cmd/labelr
