BINARY := orx
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"

.PHONY: build test lint clean install

build: lint
	go build $(LDFLAGS) -o $(BINARY) ./cmd/orx

test:
	go test -v -race -coverprofile=coverage.out ./...

lint:
	golangci-lint run

clean:
	rm -f $(BINARY) coverage.out

install:
	go install $(LDFLAGS) ./cmd/orx
