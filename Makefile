# ==============================================================================
# ZigBridge Makefile
# ==============================================================================

SHELL := /bin/bash
BIN_DIR := bin
BINARY_NAME := zigbridge
MAIN_SRC := ./cmd/zigbridge

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "1.0.0")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')

LDFLAGS := -s -w \
	-X 'main.Version=$(VERSION)' \
	-X 'main.Commit=$(COMMIT)' \
	-X 'main.BuildDate=$(BUILD_DATE)' \
	-X 'github.com/julienbreux/zigbridge/internal/version.Version=$(VERSION)' \
	-X 'github.com/julienbreux/zigbridge/internal/version.Commit=$(COMMIT)' \
	-X 'github.com/julienbreux/zigbridge/internal/version.BuildDate=$(BUILD_DATE)'

.PHONY: all
all: build

## webui: Build the web frontend distribution (Vue 3 + Tailwind CSS)
.PHONY: webui
webui:
	@echo "==> Building web UI in webui/..."
	@if [ -d "webui" ]; then \
		if [ -d "webui/node_modules" ]; then \
			if command -v npm >/dev/null 2>&1; then \
				(cd webui && npm run build); \
			elif command -v pnpm >/dev/null 2>&1; then \
				(cd webui && pnpm build); \
			fi; \
		elif [ -d "webui/dist" ] && [ -n "$$(ls -A webui/dist 2>/dev/null)" ]; then \
			echo "==> webui/node_modules not found; using pre-built webui/dist (run 'cd webui && npm install' to build from source)"; \
		elif command -v npm >/dev/null 2>&1; then \
			echo "==> Installing web UI dependencies..."; \
			(cd webui && (npm ci || npm install) && npm run build); \
		elif command -v pnpm >/dev/null 2>&1; then \
			echo "==> Installing web UI dependencies..."; \
			(cd webui && pnpm install && pnpm build); \
		else \
			echo "==> Error: webui/dist not found and neither npm nor pnpm is available" >&2; \
			exit 1; \
		fi; \
	fi

## build: Build static single binary with zero external dependencies (CGO_ENABLED=0)
.PHONY: build
build: webui
	@echo "==> Building static binary $(BIN_DIR)/$(BINARY_NAME) (CGO_ENABLED=0)..."
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -trimpath -o $(BIN_DIR)/$(BINARY_NAME) $(MAIN_SRC)
	@echo "==> Build successful: $$(ls -lh $(BIN_DIR)/$(BINARY_NAME) | awk '{print $$9, "("$$5")"}')"

## cross-compile: Build static binaries for Linux, macOS, and Windows
.PHONY: cross-compile
cross-compile: webui
	@echo "==> Cross-compiling static binaries..."
	@mkdir -p $(BIN_DIR)
	# Linux x86_64
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -trimpath -o $(BIN_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_SRC)
	# Linux ARM64 (Raspberry Pi 4 / 5, Odroid)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -trimpath -o $(BIN_DIR)/$(BINARY_NAME)-linux-arm64 $(MAIN_SRC)
	# Linux ARMv7 (Raspberry Pi 2 / 3, Orange Pi)
	GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -trimpath -o $(BIN_DIR)/$(BINARY_NAME)-linux-armv7 $(MAIN_SRC)
	# macOS Apple Silicon
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -trimpath -o $(BIN_DIR)/$(BINARY_NAME)-darwin-arm64 $(MAIN_SRC)
	# macOS Intel
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -trimpath -o $(BIN_DIR)/$(BINARY_NAME)-darwin-amd64 $(MAIN_SRC)
	# Windows x86_64
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -trimpath -o $(BIN_DIR)/$(BINARY_NAME)-windows-amd64.exe $(MAIN_SRC)
	@echo "==> Cross-compilation complete:"
	@ls -lh $(BIN_DIR)/

## test: Run all package unit tests with Go race detector
.PHONY: test
test:
	@echo "==> Running tests with race detector..."
	go test -v -race ./...

## test-coverage: Generate code coverage report
.PHONY: test-coverage
test-coverage:
	@echo "==> Running tests with coverage profile..."
	@mkdir -p coverage
	go test -v -coverprofile=coverage/coverage.out ./...
	go tool cover -html=coverage/coverage.out -o coverage/coverage.html
	@echo "==> Coverage HTML report generated at coverage/coverage.html"

## lint: Run go vet and static analysis
.PHONY: lint
lint:
	@echo "==> Running go vet..."
	go vet ./...
	@if command -v golangci-lint >/dev/null 2>&1; then \
		echo "==> Running golangci-lint..."; \
		golangci-lint run; \
	fi

## modernize: Run modern Go code analysis and transformations
.PHONY: modernize
modernize:
	@echo "==> Running modernize static checks..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run --enable=modernize; \
	else \
		echo "golangci-lint not found in PATH"; \
	fi

## vulncheck: Scan Go code and dependencies for known vulnerabilities
.PHONY: vulncheck
vulncheck:
	@echo "==> Scanning for known vulnerabilities with govulncheck..."
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

## fmt: Format all Go source files
.PHONY: fmt
fmt:
	@echo "==> Formatting code..."
	go fmt ./...

## config: Copy config.yaml.dist template to data/config.yaml
.PHONY: config
config:
	@echo "==> Creating data/ directory and copying config.yaml.dist to data/config.yaml..."
	@mkdir -p data
	cp config.yaml.dist data/config.yaml

## run: Run directly with mock transport & adapter for local testing
.PHONY: run
run:
	@echo "==> Running ZigBridge in development mode..."
	@if [ ! -f data/config.yaml ] && [ ! -f config.yaml ] && [ -f config.yaml.dist ]; then \
		echo "==> No config found, creating data/config.yaml from template..."; \
		mkdir -p data; \
		cp config.yaml.dist data/config.yaml; \
	fi
	go run $(MAIN_SRC)

## clean: Remove build artifacts and temporary files
.PHONY: clean
clean:
	@echo "==> Cleaning build artifacts..."
	rm -rf $(BIN_DIR) coverage dist

## help: Display available make targets
.PHONY: help
help:
	@echo "ZigBridge Build System"
	@echo ""
	@echo "Available commands:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'
