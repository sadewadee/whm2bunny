# Makefile for whm2bunny

# Variables
BINARY=whm2bunny
CMD_DIR=./cmd/whm2bunny
GO=go
GOFLAGS=-v
LDFLAGS=-ldflags "-w -s -X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME)"

# Version info
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT?=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME?=$(shell date -u '+%Y-%m-%d_%H:%M:%S')

# Build directories
DIST_DIR=dist
BUILD_DIR=build

# Docker variables
DOCKER_IMAGE=whm2bunny
DOCKER_TAG=$(VERSION)

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Build flags (note: ldflags variables are in commands package)
LDFLAGS=-ldflags "-w -s -X github.com/mordenhost/whm2bunny/cmd/whm2bunny/commands.Version=$(VERSION) -X github.com/mordenhost/whm2bunny/cmd/whm2bunny/commands.Commit=$(COMMIT) -X github.com/mordenhost/whm2bunny/cmd/whm2bunny/commands.BuildTime=$(BUILD_TIME)"

.PHONY: all
all: build

## build: Build the binary for current platform
.PHONY: build
build:
	@echo "Building $(BINARY)... (Version: $(VERSION), Commit: $(COMMIT))"
	$(GOBUILD) $(GOFLAGS) $(LDFLAGS) -o $(BINARY) ./$(CMD_DIR)
	@echo "Build complete: $(BINARY)"

## build-linux: Build for Linux
.PHONY: build-linux
build-linux:
	@echo "Building $(BINARY) for Linux..."
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-linux-amd64 ./$(CMD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GOBUILD) $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-linux-arm64 ./$(CMD_DIR)

## build-darwin: Build for macOS
.PHONY: build-darwin
build-darwin:
	@echo "Building $(BINARY) for macOS..."
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOBUILD) $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-darwin-amd64 ./$(CMD_DIR)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GOBUILD) $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-darwin-arm64 ./$(CMD_DIR)

## build-all: Build for all platforms
.PHONY: build-all
build-all: clean build-linux build-darwin
	@echo "All builds complete in $(BUILD_DIR)/"

## test: Run tests
.PHONY: test
test:
	@echo "Running tests..."
	$(GOTEST) -v -race -coverprofile=coverage.out ./...
	@echo "Coverage report: coverage.out"

## test-coverage: Generate HTML coverage report
.PHONY: test-coverage
test-coverage: test
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## clean: Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -f $(BINARY)
	rm -rf $(BUILD_DIR)
	rm -rf $(DIST_DIR)
	rm -f coverage.out coverage.html
	@echo "Clean complete"

## lint: Run linter
.PHONY: lint
lint:
	@echo "Running linters..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed. Installing..."; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
		golangci-lint run ./...; \
	fi

## fmt: Format code
.PHONY: fmt
fmt:
	@echo "Formatting code..."
	$(GOCMD) fmt ./...

## vet: Run go vet
.PHONY: vet
vet:
	@echo "Running go vet..."
	$(GOCMD) vet ./...

## deps: Download dependencies
.PHONY: deps
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

## docker-build: Build Docker image
.PHONY: docker-build
docker-build:
	@echo "Building Docker image..."
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_TIME=$(BUILD_TIME) \
		-t $(DOCKER_IMAGE):$(DOCKER_TAG) \
		-t $(DOCKER_IMAGE):latest \
		.
	@echo "Docker image built: $(DOCKER_IMAGE):$(DOCKER_TAG)"

## docker-push: Push Docker image to registry
.PHONY: docker-push
docker-push: docker-build
	@echo "Pushing Docker image..."
	docker push $(DOCKER_IMAGE):$(DOCKER_TAG)
	docker push $(DOCKER_IMAGE):latest

## docker-run: Run container with docker-compose
.PHONY: docker-run
docker-run:
	@echo "Starting containers..."
	docker-compose up -d

## docker-stop: Stop containers
.PHONY: docker-stop
docker-stop:
	@echo "Stopping containers..."
	docker-compose down

## docker-logs: View container logs
.PHONY: docker-logs
docker-logs:
	docker-compose logs -f whm2bunny

## docker-ps: List containers
.PHONY: docker-ps
docker-ps:
	docker-compose ps

## docker-restart: Restart containers
.PHONY: docker-restart
docker-restart:
	@echo "Restarting containers..."
	docker-compose restart whm2bunny

## install: Run installation script
.PHONY: install
install:
	@echo "Running installation script..."
	./scripts/install.sh

## config-generate: Generate default config
.PHONY: config-generate
config-generate:
	@echo "Generating default config..."
	./$(BINARY) config generate config.yaml

## config-validate: Validate config
.PHONY: config-validate
config-validate:
	@echo "Validating config..."
	./$(BINARY) config validate config.yaml

## run: Run the binary locally
.PHONY: run
run: build
	@echo "Starting $(BINARY)..."
	./$(BINARY) serve

## release: Create release artifacts
.PHONY: release
release: clean
	@echo "Creating release artifacts..."
	@mkdir -p $(DIST_DIR)

	# Linux builds
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY)-linux-amd64 ./$(CMD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY)-linux-arm64 ./$(CMD_DIR)

	# macOS builds
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY)-darwin-amd64 ./$(CMD_DIR)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY)-darwin-arm64 ./$(CMD_DIR)

	# Create checksums
	cd $(DIST_DIR) && sha256sum * > SHA256SUMS

	@echo "Release artifacts created in $(DIST_DIR)/"

## help: Show this help message
.PHONY: help
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Available targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## /  /' | column -t -s ':'

## ci: Run CI checks (fmt, vet, lint, test)
.PHONY: ci
ci: fmt vet lint test
	@echo "CI checks passed!"

## dev: Development setup
.PHONY: dev
dev:
	@echo "Setting up development environment..."
	$(GOMOD) download
	@echo "Development environment ready!"
	@echo "Run 'make run' to start the application"
