.PHONY: build clean install test run-dev build-all

# Binary name
BINARY_NAME=nacos-cli

# Build directory
BUILD_DIR=build

# Version
VERSION?=0.0.1

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Build the project
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) -v

# Build for all platforms
build-all: build-linux build-darwin build-windows

build-linux:
	@echo "Building for Linux..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME)-$(VERSION)-linux-amd64 -v

build-darwin:
	@echo "Building for macOS..."
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME)-$(VERSION)-darwin-amd64 -v
	GOOS=darwin GOARCH=arm64 $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME)-$(VERSION)-darwin-arm64 -v

build-windows:
	@echo "Building for Windows..."
	@mkdir -p $(BUILD_DIR)
	GOOS=windows GOARCH=amd64 $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME)-$(VERSION)-windows-amd64.exe -v

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@$(GOCLEAN)
	@rm -rf $(BUILD_DIR)

# Install dependencies
deps:
	@echo "Downloading dependencies..."
	@$(GOMOD) download
	@$(GOMOD) tidy

# Run tests
test:
	@echo "Running tests..."
	@$(GOTEST) -v ./...

# Run integration tests
test-integration:
	@echo "Running integration tests..."
	@./test.sh

# Install the binary
install: build
	@echo "Installing $(BINARY_NAME)..."
	@cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/

# Run in development mode
run-dev:
	@$(GOCMD) run main.go

# Show help
help:
	@echo "Nacos CLI Build Commands:"
	@echo "  make build             - Build the binary"
	@echo "  make build-all         - Build for all platforms"
	@echo "  make clean             - Clean build artifacts"
	@echo "  make deps              - Download dependencies"
	@echo "  make test              - Run unit tests"
	@echo "  make test-integration  - Run integration tests"
	@echo "  make install           - Install the binary"
	@echo "  make run-dev           - Run in development mode"
