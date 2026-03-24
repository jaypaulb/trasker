// internal/client/webui/embed.go
package webui

import "embed"

// Assets holds the built Svelte SPA files.
// The build step must run before Go compilation.
//
//go:embed all:static
var Assets embed.FS
