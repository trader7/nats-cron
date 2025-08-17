.PHONY: build clean test fmt vet install dev docker help

# Variables
BINARY_SERVER=nats-cron-server
BINARY_CLI=nats-cron
VERSION?=$(shell git describe --tags --always --dirty)
BUILD_TIME?=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-X main.version=${VERSION} -X main.buildTime=${BUILD_TIME}"

# Default target
all: build

## Build binaries
build: build-server build-cli

build-server:
	@echo "Building server..."
	go build ${LDFLAGS} -o bin/${BINARY_SERVER} ./cmd/nats-cron-server

build-cli:
	@echo "Building CLI..."
	go build ${LDFLAGS} -o bin/${BINARY_CLI} ./cmd/nats-cron

## Install binaries to $GOPATH/bin
install:
	@echo "Installing binaries..."
	go install ${LDFLAGS} ./cmd/nats-cron-server
	go install ${LDFLAGS} ./cmd/nats-cron

## Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf bin/
	go clean ./...

## Run tests
test:
	@echo "Running tests..."
	go test -v ./...

## Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...

## Run go vet
vet:
	@echo "Running go vet..."
	go vet ./...

## Run go mod tidy
tidy:
	@echo "Tidying modules..."
	go mod tidy

## Lint code (requires golangci-lint)
lint:
	@echo "Running linter..."
	golangci-lint run

## Development mode - run server with auto-restart
dev:
	@echo "Starting development server..."
	go run ./cmd/nats-cron-server

## Create example job file
example:
	@mkdir -p examples
	@echo "Creating example job file..."
	@echo "Example job created at examples/sample-job.json"

## Docker build
docker:
	@echo "Building Docker image..."
	docker build -t nats-cron:${VERSION} .
	docker build -t nats-cron:latest .

## Show help
help:
	@echo "Available commands:"
	@echo "  build       - Build both server and CLI binaries"
	@echo "  build-server- Build server binary only"
	@echo "  build-cli   - Build CLI binary only"
	@echo "  install     - Install binaries to GOPATH/bin"
	@echo "  clean       - Clean build artifacts"
	@echo "  test        - Run tests"
	@echo "  fmt         - Format code"
	@echo "  vet         - Run go vet"
	@echo "  tidy        - Run go mod tidy"
	@echo "  lint        - Run linter (requires golangci-lint)"
	@echo "  dev         - Run development server"
	@echo "  example     - Create example job file"
	@echo "  docker      - Build Docker image"
	@echo "  help        - Show this help"