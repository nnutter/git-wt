package main

import (
	"context"
	"os"

	"charm.land/fang/v2"
	"github.com/nnutter/git-wt/internal/gitwt"
)

func main() {
	ctx := context.Background()
	if err := fang.Execute(ctx, gitwt.RootCommand); err != nil {
		os.Exit(1)
	}
}
