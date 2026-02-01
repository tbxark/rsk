BUILD_DIR=./build
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE=$(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
CURRENT_OS := $(shell uname -s | tr '[:upper:]' '[:lower:]')
CURRENT_ARCH := $(shell uname -m | tr '[:upper:]' '[:lower:]')

LD_FLAGS=-ldflags "\
	-s -w \
	-X github.com/tbxark/rsk/pkg/rsk/version.Version=$(VERSION) \
	-X github.com/tbxark/rsk/pkg/rsk/version.GitCommit=$(GIT_COMMIT) \
	-X github.com/tbxark/rsk/pkg/rsk/version.BuildDate=$(BUILD_DATE)"

GO_BUILD=CGO_ENABLED=0 go build $(LD_FLAGS)

.PHONY: all
all: build

.PHONY: build
build:
	@mkdir -p $(BUILD_DIR)
	$(GO_BUILD) -o $(BUILD_DIR)/rsk-server ./cmd/rsk-server
	$(GO_BUILD) -o $(BUILD_DIR)/rsk-client ./cmd/rsk-client

.PHONY: build-linux
build-linux:
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GO_BUILD) -o $(BUILD_DIR)/rsk-server-linux-amd64 ./cmd/rsk-server
	GOOS=linux GOARCH=amd64 $(GO_BUILD) -o $(BUILD_DIR)/rsk-client-linux-amd64 ./cmd/rsk-client

.PHONY: build-linux-arm64
build-linux-arm64:
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=arm64 $(GO_BUILD) -o $(BUILD_DIR)/rsk-server-linux-arm64 ./cmd/rsk-server
	GOOS=linux GOARCH=arm64 $(GO_BUILD) -o $(BUILD_DIR)/rsk-client-linux-arm64 ./cmd/rsk-client

.PHONY: build-darwin
build-darwin:
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=amd64 $(GO_BUILD) -o $(BUILD_DIR)/rsk-server-darwin-amd64 ./cmd/rsk-server
	GOOS=darwin GOARCH=amd64 $(GO_BUILD) -o $(BUILD_DIR)/rsk-client-darwin-amd64 ./cmd/rsk-client
	GOOS=darwin GOARCH=arm64 $(GO_BUILD) -o $(BUILD_DIR)/rsk-server-darwin-arm64 ./cmd/rsk-server
	GOOS=darwin GOARCH=arm64 $(GO_BUILD) -o $(BUILD_DIR)/rsk-client-darwin-arm64 ./cmd/rsk-client

.PHONY: build-windows
build-windows:
	@mkdir -p $(BUILD_DIR)
	GOOS=windows GOARCH=amd64 $(GO_BUILD) -o $(BUILD_DIR)/rsk-server-windows-amd64.exe ./cmd/rsk-server
	GOOS=windows GOARCH=amd64 $(GO_BUILD) -o $(BUILD_DIR)/rsk-client-windows-amd64.exe ./cmd/rsk-client

.PHONY: build-all
build-all: build-linux build-linux-arm64 build-darwin build-windows

.PHONY: install
install:
	go install $(LD_FLAGS) ./cmd/rsk-server
	go install $(LD_FLAGS) ./cmd/rsk-client

.PHONY: test
test:
	go test -v -race -cover ./...

.PHONY: test-coverage
test-coverage:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

.PHONY: clean
clean:
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

.PHONY: format
format:
	go fmt ./...
	go mod tidy

.PHONY: lint
lint:
	golangci-lint run

.PHONY: release
release:
	goreleaser release --clean

.PHONY: release-snapshot
release-snapshot:
	goreleaser release --snapshot --clean

.PHONY: help
help:
	@echo "Available targets:"
	@echo "  all              - Build server and client for current platform (default)"
	@echo "  build            - Build server and client for current platform"
	@echo "  build-linux      - Build for Linux AMD64"
	@echo "  build-linux-arm64 - Build for Linux ARM64"
	@echo "  build-darwin     - Build for macOS (AMD64 and ARM64)"
	@echo "  build-windows    - Build for Windows AMD64"
	@echo "  build-all        - Build for all platforms"
	@echo "  install          - Install binaries to GOPATH/bin"
	@echo "  test             - Run tests with race detector"
	@echo "  test-coverage    - Run tests with coverage report"
	@echo "  clean            - Remove build artifacts"
	@echo "  format           - Format code and tidy dependencies"
	@echo "  lint             - Run linter"
	@echo "  release          - Create a release with goreleaser"
	@echo "  release-snapshot - Create a snapshot release"
	@echo "  help             - Show this help message"