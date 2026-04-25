package main

import (
	"context"
	"os"

	"charm.land/fang/v2"
	"github.com/nnutter/git-wt/internal/gitwt"
)

func main() {
	if err := fang.Execute(context.Background(), gitwt.Command); err != nil {
		os.Exit(1)
	}
}
