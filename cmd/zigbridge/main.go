package main

import (
	"fmt"
	"os"

	"github.com/julienbreux/zigbridge/internal/cli"
	"github.com/julienbreux/zigbridge/internal/version"
)

// Build-time variables injected via -ldflags
var (
	Version   = "1.0.0"
	Commit    = "unknown"
	BuildDate = "unknown"
)

func init() {
	version.Version = Version
	version.Commit = Commit
	version.BuildDate = BuildDate
}

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
