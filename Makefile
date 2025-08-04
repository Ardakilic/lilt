# FLAC to 16-bit Converter - Makefile
# Build cross-platform binaries

BINARY_NAME=flac-converter
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

.PHONY: build clean test fmt lint build-all package install uninstall
