package main

import (
	"fmt"

	"github.com/jaypaulb/trasker/internal/shared/version"
)

func main() {
	fmt.Printf("trasker-server %s (commit: %s)\n", version.Version, version.Commit)
}
