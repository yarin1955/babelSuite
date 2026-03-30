package main

import (
	"context"
	"os"

	"github.com/babelsuite/babelsuite/cli/babelctl"
)

func main() {
	runner := babelctl.NewRunner(os.Stdout, os.Stderr)
	os.Exit(runner.Run(context.Background(), os.Args[1:]))
}
