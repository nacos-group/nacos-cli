# Mark targets as phony (not actual files)
.PHONY: help \
	build build-linux build-darwin build-windows build-all package-all \
	clean deps test test-integration install run-dev

# Set default target when running 'make' without arguments
.DEFAULT_GOAL := build

# Binary name
BINARY_NAME=nacos-cli

# Build directory
BUILD_DIR=build

# Version
VERSION?=1.0.4

# Git info
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE=$(shell date -u '+%Y-%m-%dT%H:%M:%SZ')

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
LDFLAGS=-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

help: ## Show help information
	@echo "Nacos CLI Build Commands:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z0-9._-]+:.*?## / {printf "\033[36m  make %-20s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the binary
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) -v

build-linux: ## Build for Linux (amd64, arm64)
	@echo "Building for Linux..."
	@mkdir -p $(BUILD_DIR)/$(VERSION)
	GOOS=linux GOARCH=amd64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-$(VERSION)-linux-amd64 -v
	GOOS=linux GOARCH=arm64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-$(VERSION)-linux-arm64 -v

build-darwin: ## Build for macOS (amd64, arm64)
	@echo "Building for macOS..."
	@mkdir -p $(BUILD_DIR)/$(VERSION)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-$(VERSION)-darwin-amd64 -v
	GOOS=darwin GOARCH=arm64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-$(VERSION)-darwin-arm64 -v

build-windows: ## Build for Windows (amd64, arm64)
	@echo "Building for Windows..."
	@mkdir -p $(BUILD_DIR)/$(VERSION)
	GOOS=windows GOARCH=amd64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-$(VERSION)-windows-amd64.exe -v
	GOOS=windows GOARCH=arm64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-$(VERSION)-windows-arm64.exe -v

build-all: build-linux build-darwin build-windows package-all ## Build for all platforms

package-all: ## Package all binaries into zip files
	@echo "Packaging binaries..."
	@mkdir -p $(BUILD_DIR)/$(VERSION)
	@cd $(BUILD_DIR) && for f in $(BINARY_NAME)-$(VERSION)-linux-amd64 $(BINARY_NAME)-$(VERSION)-linux-arm64 $(BINARY_NAME)-$(VERSION)-darwin-amd64 $(BINARY_NAME)-$(VERSION)-darwin-arm64; do \
		zip "$(VERSION)/$$f.zip" "$$f"; \
	done
	@cd $(BUILD_DIR) && zip "$(VERSION)/$(BINARY_NAME)-$(VERSION)-windows-amd64.zip" "$(BINARY_NAME)-$(VERSION)-windows-amd64.exe"
	@cd $(BUILD_DIR) && zip "$(VERSION)/$(BINARY_NAME)-$(VERSION)-windows-arm64.zip" "$(BINARY_NAME)-$(VERSION)-windows-arm64.exe"
	@echo "Packaged to $(BUILD_DIR)/$(VERSION)/"

clean: ## Clean build artifacts
	@echo "Cleaning..."
	@$(GOCLEAN)
	@rm -rf $(BUILD_DIR)

deps: ## Download dependencies
	@echo "Downloading dependencies..."
	@$(GOMOD) download
	@$(GOMOD) tidy

test: ## Run unit tests
	@echo "Running tests..."
	@$(GOTEST) -v ./...

test-integration: ## Run integration tests
	@echo "Running integration tests..."
	@./test.sh

install: build ## Install the binary to /usr/local/bin
	@echo "Installing $(BINARY_NAME)..."
	@cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/

run-dev: ## Run in development mode
	@$(GOCMD) run main.go
