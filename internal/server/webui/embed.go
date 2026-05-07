// Package webui embeds the built dashboard SPA so the Go server can serve it
// directly (no nginx/Caddy in production).
package webui

import "embed"

// Assets holds the built dashboard SPA files (web/server-ui/build/).
// The Dockerfile copies build output into static/ before go build.
//
//go:embed all:static
var Assets embed.FS
