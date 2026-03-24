#!/bin/sh
set -euo pipefail

# Required environment variables
: "${GOOS:?GOOS is required}"
: "${GOARCH:?GOARCH is required}"
: "${API_KEY:?API_KEY is required}"
: "${SERVER_URL:?SERVER_URL is required}"
: "${VERSION:=dev}"

# Determine binary extension
EXT=""
if [ "$GOOS" = "windows" ]; then
  EXT=".exe"
fi

OUTPUT="/out/trasker-client-${GOOS}-${GOARCH}${EXT}"

echo "Building trasker-client for ${GOOS}/${GOARCH}..."

CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" go build \
  -ldflags="-s -w -X main.apiKey=${API_KEY} -X main.serverURL=${SERVER_URL} -X main.version=${VERSION}" \
  -trimpath \
  -o "$OUTPUT" \
  ./cmd/trasker-client

echo "Built: $OUTPUT"
ls -lh "$OUTPUT"
