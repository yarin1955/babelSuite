package main

import (
	"context"
	"os"

	babelctl "github.com/babelsuite/babelsuite/cli/ctl"
)

func main() {
	runner := babelctl.NewRunner(os.Stdout, os.Stderr)
	os.Exit(runner.Run(context.Background(), os.Args[1:]))
}
