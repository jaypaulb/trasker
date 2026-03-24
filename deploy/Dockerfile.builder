# ============================================
# Trasker Client Builder
# ============================================
# This container provides the Go toolchain for cross-compiling
# client binaries. It is invoked by the API server's builder package
# when a user requests a client download.
#
# Key design choice: modernc.org/sqlite is pure Go, so we do NOT need
# CGo or platform-specific C toolchains. CGO_ENABLED=0 works for all targets.
#
# Usage (invoked by builder.go, not directly):
#   docker run --rm \
#     -v /path/to/trasker:/src:ro \
#     -v /tmp/trasker-builds:/out \
#     -e GOOS=linux -e GOARCH=amd64 \
#     -e API_KEY=xxx -e SERVER_URL=https://... -e VERSION=1.0.0 \
#     trasker-builder

FROM golang:1.23-alpine

RUN apk add --no-cache git

WORKDIR /src

# Pre-download modules for faster builds.
# This layer is cached — only re-runs when go.mod/go.sum change.
COPY go.mod go.sum ./
RUN go mod download

# Copy full source (needed for build)
COPY . .

# Build script that reads env vars and runs go build
COPY deploy/builder-entrypoint.sh /usr/local/bin/builder-entrypoint.sh
RUN chmod +x /usr/local/bin/builder-entrypoint.sh

ENTRYPOINT ["builder-entrypoint.sh"]
