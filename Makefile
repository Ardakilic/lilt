# Lilt - FLAC to 16-bit Converter - Makefile
# Build cross-platform binaries

BINARY_NAME=lilt
VERSION=$(shell git describe --tags --always --dirty)
LDFLAGS=-ldflags="-s -w -X main.version=$(VERSION)"

# Default build
build:
	go build $(LDFLAGS) -o $(BINARY_NAME) .

# Clean build artifacts
clean:
	rm -rf dist/
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_NAME).exe

# Test
test:
	go test -v ./...

# Format code
fmt:
	go fmt ./...

# Lint code
lint:
	golangci-lint run

# Build for all platforms
build-all: clean
	mkdir -p dist
	
	# Linux builds
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-linux-amd64 .
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-linux-arm64 .
	GOOS=linux GOARCH=386 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-linux-386 .
	GOOS=linux GOARCH=arm go build $(LDFLAGS) -o dist/$(BINARY_NAME)-linux-arm .
	
	# Windows builds
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-windows-amd64.exe .
	GOOS=windows GOARCH=arm64 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-windows-arm64.exe .
	GOOS=windows GOARCH=386 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-windows-386.exe .
	
	# macOS builds
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-darwin-arm64 .

# Create archives
package: build-all
	cd dist && \
	for file in $(BINARY_NAME)-linux-* $(BINARY_NAME)-darwin-*; do \
		if [ -f "$$file" ]; then \
			tar -czf "$$file.tar.gz" "$$file"; \
		fi; \
	done && \
	for file in $(BINARY_NAME)-windows-*.exe; do \
		if [ -f "$$file" ]; then \
			zip "$${file%.exe}.zip" "$$file"; \
		fi; \
	done

# Install locally
install: build
	cp $(BINARY_NAME) /usr/local/bin/

# Uninstall
uninstall:
	rm -f /usr/local/bin/$(BINARY_NAME)

# Help
help:
	@echo "Lilt Makefile Commands:"
	@echo ""
	@echo "Build:"
	@echo "  build              Build for current platform"
	@echo "  build-all         Build for all platforms"
	@echo "  clean             Clean build artifacts"
	@echo "  package           Create distribution archives"
	@echo ""
	@echo "Development:"
	@echo "  test              Run tests"
	@echo "  fmt               Format code"
	@echo "  lint              Lint code"
	@echo ""
	@echo "Installation:"
	@echo "  install           Install locally"
	@echo "  uninstall         Uninstall"
	@echo ""
	@echo "Docker Build:"
	@echo "  build-docker           Build using golang:1.26.4-trixie"
	@echo "  build-all-docker       Build all platforms using Docker"
	@echo "  install-deps-docker   Download dependencies using Docker"
	@echo "  test-docker           Run tests using Docker"
	@echo "  fmt-docker           Format code using Docker"
	@echo "  lint-docker          Lint code using Docker"
	@echo ""
	@echo "Serena MCP:"
	@echo "  serena-up         Start Serena MCP service"
	@echo "  serena-down       Stop Serena MCP service"
	@echo "  serena-build     Build/rebuild Serena Docker image"
	@echo "  serena-logs      View Serena logs"
	@echo "  serena-index     Index the project workspace"
	@echo "  serena-health    Health check the project workspace"

# ── Serena MCP ──────────────────────────────────────────────────────────

serena-up: ## Start Serena MCP service
	docker compose --profile serena up serena -d

serena-down: ## Down Serena MCP service (removes container)
	docker compose --profile serena down serena

serena-build: ## Build/rebuild Serena Docker image
	docker compose --profile serena build serena

serena-logs: ## Follow Serena logs
	docker compose --profile serena logs -f serena

serena-index: ## Index project with Serena
	docker compose --profile serena exec serena serena project index /workspace/lilt

serena-health: ## Check Serena health
	@http_status=$$(curl -s -o /dev/null -w "%{http_code}" --max-time 5 --connect-timeout 2 http://localhost:10123/sse 2>/dev/null || true); \
	if [ "$${http_status}" = "200" ]; then echo "✓ Serena is healthy"; else echo "✗ Serena is not responding"; fi

# ============================================
# Docker Build Commands
# ============================================

GO_IMAGE=golang:1.26.4-trixie

build-docker:
	docker run --rm -it -v $$(pwd):/src $(GO_IMAGE) \
		sh -c "go build $(LDFLAGS) -o $(BINARY_NAME) ."
	@chmod +x $(BINARY_NAME)

build-all-docker: clean
	docker run --rm -v $$(pwd):/src $(GO_IMAGE) \
		sh -c "mkdir -p dist && \
		GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-linux-amd64 . && \
		GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-linux-arm64 . && \
		GOOS=linux GOARCH=386 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-linux-386 . && \
		GOOS=linux GOARCH=arm go build $(LDFLAGS) -o dist/$(BINARY_NAME)-linux-arm ."

install-deps-docker:
	docker run --rm -v $$(pwd):/src $(GO_IMAGE) \
		sh -c "go mod download"

test-docker:
	docker run --rm -v $$(pwd):/src $(GO_IMAGE) \
		sh -c "go test -v ./..."

fmt-docker:
	docker run --rm -v $$(pwd):/src $(GO_IMAGE) \
		sh -c "go fmt ./..."

lint-docker:
	docker run --rm -v $$(pwd):/src $(GO_IMAGE) \
		sh -c "golangci-lint run || true"

.PHONY: build clean test fmt lint build-all package install uninstall serena-up serena-down serena-build serena-logs serena-index serena-health help build-docker build-all-docker install-deps-docker test-docker fmt-docker lint-docker
