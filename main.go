package main

import (
	"context"
	"os"

	"charm.land/fang/v2"

	"github.com/nnutter/timber/internal/timber"
)

// Version is set via ldflags at build time (e.g. Homebrew, GoReleaser).
var Version string

func main() {
	if err := fang.Execute(
		context.Background(),
		timber.Command,
		fang.WithVersion(Version),
	); err != nil {
		os.Exit(1)
	}
}
