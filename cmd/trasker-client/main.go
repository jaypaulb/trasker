package main

import (
	"fmt"

	"github.com/jaypaulb/trasker/internal/shared/version"
)

func main() {
	fmt.Printf("trasker-client %s (commit: %s)\n", version.Version, version.Commit)
}
