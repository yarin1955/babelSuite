package demofs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	EnableEnvVar = "BABELSUITE_ENABLE_DEMO"
)

type Manifest struct {
	SuitesFile     string `yaml:"suitesFile"`
	ProfilesFile   string `yaml:"profilesFile"`
	StdlibFile     string `yaml:"stdlibFile"`
	ExecutionsFile string `yaml:"executionsFile"`
}

func Enabled() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(EnableEnvVar)))
	switch value {
	case "", "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

func ResolveRoot() string {
	return ResolveRootFromRepo(resolveRepoRoot())
}

func ResolveRootFromRepo(repoRoot string) string {
	return filepath.Join(repoRoot, "demo")
}

func LoadManifest() (Manifest, error) {
	var manifest Manifest
	data, err := os.ReadFile(filepath.Join(ResolveRoot(), "manifest.yaml"))
	if err != nil {
		return manifest, err
	}
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return manifest, err
	}
	return manifest, nil
}

func LoadJSON[T any](relativePath string) (T, error) {
	var value T
	data, err := os.ReadFile(filepath.Join(ResolveRoot(), filepath.FromSlash(strings.TrimSpace(relativePath))))
	if err != nil {
		return value, err
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return value, err
	}
	return value, nil
}

func resolveRepoRoot() string {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	return filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", ".."))
}
