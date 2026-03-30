package examplefs

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const RootEnvVar = "BABELSUITE_EXAMPLES_ROOT"

func ResolveRoot() string {
	return ResolveRootFromRepo(resolveRepoRoot())
}

func ResolveRootFromRepo(repoRoot string) string {
	if configured := strings.TrimSpace(os.Getenv(RootEnvVar)); configured != "" {
		if absolute, err := filepath.Abs(configured); err == nil {
			return absolute
		}
		return configured
	}
	return filepath.Join(repoRoot, "examples")
}

func SuiteFilePath(suiteID, path string) string {
	return filepath.Join(ResolveRoot(), "oci-suites", suiteID, filepath.FromSlash(path))
}

func resolveRepoRoot() string {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	return filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", ".."))
}
