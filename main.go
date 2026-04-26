package main

import (
"context"
"os"

"charm.land/fang/v2"
"github.com/nnutter/git-wt/internal/gitwt"
)

func main() {
	cmd := gitwt.Root()
	if err := fang.Execute(context.Background(), cmd); err != nil {
		os.Exit(1)
	}
}