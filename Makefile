# Trasker Makefile
# Builds client and server binaries for all supported platforms.
# Version info is stamped via ldflags.

MODULE := github.com/jaypaulb/trasker
VERSION_PKG := $(MODULE)/internal/shared/version

# Version defaults — override with: make VERSION=v1.0.0 COMMIT=abc1234
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# ldflags for version stamping
LDFLAGS := -X $(VERSION_PKG).Version=$(VERSION) -X $(VERSION_PKG).Commit=$(COMMIT)

# Output directory
DIST := dist

# Build targets: os/arch pairs
CLIENT_TARGETS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64
SERVER_TARGETS := linux/amd64 linux/arm64

.PHONY: all clean test vet client server client-all server-all

## Default: build client and server for the current platform
all: client server

## Build client for current platform
client:
	go build -ldflags '$(LDFLAGS)' -o $(DIST)/trasker-client ./cmd/trasker-client/

## Build server for current platform
server:
	go build -ldflags '$(LDFLAGS)' -o $(DIST)/trasker-server ./cmd/trasker-server/

## Build client for all platforms
client-all:
	@for target in $(CLIENT_TARGETS); do \
		os=$${target%/*}; \
		arch=$${target#*/}; \
		ext=""; \
		if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		echo "Building client: $$os/$$arch"; \
		GOOS=$$os GOARCH=$$arch go build -ldflags '$(LDFLAGS)' \
			-o $(DIST)/trasker-client-$$os-$$arch$$ext ./cmd/trasker-client/ || exit 1; \
	done

## Build server for all platforms
server-all:
	@for target in $(SERVER_TARGETS); do \
		os=$${target%/*}; \
		arch=$${target#*/}; \
		echo "Building server: $$os/$$arch"; \
		GOOS=$$os GOARCH=$$arch go build -ldflags '$(LDFLAGS)' \
			-o $(DIST)/trasker-server-$$os-$$arch ./cmd/trasker-server/ || exit 1; \
	done

## Build client with baked-in API key and server URL (used by server build pipeline)
## Usage: make client-stamped API_KEY=tsk_... SERVER_URL=https://... GOOS=linux GOARCH=amd64
client-stamped:
ifndef API_KEY
	$(error API_KEY is required for client-stamped)
endif
ifndef SERVER_URL
	$(error SERVER_URL is required for client-stamped)
endif
	GOOS=$(or $(GOOS),linux) GOARCH=$(or $(GOARCH),amd64) go build \
		-ldflags '$(LDFLAGS) -X $(VERSION_PKG).APIKey=$(API_KEY) -X $(VERSION_PKG).ServerURL=$(SERVER_URL)' \
		-o $(DIST)/trasker-client-$(GOOS)-$(GOARCH) ./cmd/trasker-client/

## Run all tests
test:
	go test ./...

## Run go vet
vet:
	go vet ./...

## Remove build artifacts
clean:
	rm -rf $(DIST)

## Show version info that would be stamped
version:
	@echo "Version: $(VERSION)"
	@echo "Commit:  $(COMMIT)"
