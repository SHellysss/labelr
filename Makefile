include .env
export

LDFLAGS := -X 'github.com/pankajbeniwal/labelr/internal/gmail.ClientID=$(GOOGLE_CLIENT_ID)' \
           -X 'github.com/pankajbeniwal/labelr/internal/gmail.ClientSecret=$(GOOGLE_CLIENT_SECRET)'

.PHONY: build run test clean release

build:
	go build -ldflags "$(LDFLAGS)" -o bin/labelr ./cmd/labelr

run:
	go run -ldflags "$(LDFLAGS)" ./cmd/labelr

test:
	go test ./... -v

clean:
	rm -rf bin/

release:
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/labelr-darwin-arm64 ./cmd/labelr
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/labelr-darwin-amd64 ./cmd/labelr
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/labelr-linux-amd64 ./cmd/labelr
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/labelr-windows-amd64.exe ./cmd/labelr
