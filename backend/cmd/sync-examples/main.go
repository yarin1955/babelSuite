package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/babelsuite/babelsuite/internal/examplefs"
	"github.com/babelsuite/babelsuite/internal/examples"
)

func main() {
	repoRoot, err := resolveRepoRoot(os.Args[1:])
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	written, err := examples.SyncWorkspace(repoRoot)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	_, _ = fmt.Fprintf(os.Stdout, "synced %d example files into %s\n", written, examplefs.ResolveRootFromRepo(repoRoot))
}

func resolveRepoRoot(args []string) (string, error) {
	if len(args) > 1 {
		return "", fmt.Errorf("usage: sync-examples [repo-root]")
	}
	if len(args) == 1 {
		return filepath.Abs(args[0])
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if filepath.Base(cwd) == "backend" {
		return filepath.Dir(cwd), nil
	}
	return cwd, nil
}
