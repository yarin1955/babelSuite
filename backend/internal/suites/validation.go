package suites

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	maxSuiteSourceFiles = 512
	maxSuiteSourceBytes = 2 << 20
)

var allowedSuiteRootFiles = map[string]struct{}{
	"suite.star":             {},
	"metadata.yaml":          {},
	"metadata.yml":           {},
	"README.md":              {},
	"dependencies.yaml":      {},
	"dependencies.yml":       {},
	"dependencies.lock.yaml": {},
	"dependencies.lock.yml":  {},
}

var allowedSuiteFolders = map[string]struct{}{
	"api":        {},
	"artifacts":  {},
	"certs":      {},
	"compat":     {},
	"dashboards": {},
	"fixtures":   {},
	"gateway":    {},
	"load":       {},
	"mock":       {},
	"policies":   {},
	"profiles":   {},
	"scenarios":  {},
	"scripts":    {},
	"service":    {},
	"services":   {},
	"tasks":      {},
	"tests":      {},
	"traffic":    {},
	"resources":  {},
	"sql":        {},
}

var allowedSuiteExtensions = map[string]struct{}{
	".csv":     {},
	".cue":     {},
	".feature": {},
	".go":      {},
	".gql":     {},
	".graphql": {},
	".hurl":    {},
	".jmx":     {},
	".js":      {},
	".json":    {},
	".md":      {},
	".ndjson":  {},
	".proto":   {},
	".ps1":     {},
	".py":      {},
	".rego":    {},
	".sh":      {},
	".sql":     {},
	".star":    {},
	".ts":      {},
	".txt":     {},
	".wsdl":    {},
	".xml":     {},
	".xsd":     {},
	".yaml":    {},
	".yml":     {},
}

var blockedSuiteExtensions = map[string]struct{}{
	".7z":    {},
	".bin":   {},
	".com":   {},
	".dll":   {},
	".dmg":   {},
	".exe":   {},
	".gz":    {},
	".iso":   {},
	".jar":   {},
	".msi":   {},
	".rar":   {},
	".so":    {},
	".tar":   {},
	".tgz":   {},
	".xz":    {},
	".zip":   {},
	".dylib": {},
}

func ValidateDefinition(suite Definition) error {
	if strings.TrimSpace(suite.ID) == "" {
		return fmt.Errorf("suite ID is required")
	}
	if strings.TrimSpace(suite.SuiteStar) == "" {
		return fmt.Errorf("suite.star is required")
	}
	if len(suite.SourceFiles) > maxSuiteSourceFiles {
		return fmt.Errorf("suite has too many source files (%d > %d)", len(suite.SourceFiles), maxSuiteSourceFiles)
	}

	seenPaths := make(map[string]struct{}, len(suite.SourceFiles))
	for _, file := range suite.SeedSources {
		normalizedPath, err := validateSuiteSourcePath(file.Path)
		if err != nil {
			return err
		}
		if err := validateSuiteSourcePlacement(normalizedPath); err != nil {
			return err
		}
	}

	for _, folder := range suite.Folders {
		if err := validateSuiteFolder(folder); err != nil {
			return err
		}
	}

	for _, file := range suite.SourceFiles {
		normalizedPath, err := validateSuiteSourcePath(file.Path)
		if err != nil {
			return err
		}
		if _, exists := seenPaths[normalizedPath]; exists {
			return fmt.Errorf("duplicate source path %q", normalizedPath)
		}
		seenPaths[normalizedPath] = struct{}{}
		if err := validateSuiteSourcePlacement(normalizedPath); err != nil {
			return err
		}
		if err := validateSuiteSourceContent(normalizedPath, file.Content); err != nil {
			return err
		}
	}

	return nil
}

func validateSuiteFolder(folder FolderEntry) error {
	name := strings.TrimSpace(folder.Name)
	if _, ok := allowedSuiteFolders[name]; !ok {
		return fmt.Errorf("top-level folder %q is not allowed in a suite package", name)
	}
	for _, file := range folder.Files {
		normalized, err := validateSuiteSourcePath(normalizeSourcePath(name, file))
		if err != nil {
			return err
		}
		if !strings.HasPrefix(normalized, name+"/") {
			return fmt.Errorf("file %q escapes the %q folder", file, name)
		}
		if err := validateSuiteSourcePlacement(normalized); err != nil {
			return err
		}
	}
	return nil
}

func validateSuiteSourcePlacement(filePath string) error {
	topLevel, _, hasNestedPath := strings.Cut(filePath, "/")
	if !hasNestedPath {
		if _, ok := allowedSuiteRootFiles[filePath]; ok {
			return nil
		}
		return fmt.Errorf("root file %q is not allowed in a suite package", filePath)
	}
	if _, ok := allowedSuiteFolders[topLevel]; ok {
		return nil
	}
	return fmt.Errorf("top-level folder %q is not allowed in a suite package", topLevel)
}

func validateSuiteSourcePath(filePath string) (string, error) {
	trimmed := strings.TrimSpace(strings.ReplaceAll(filePath, "\\", "/"))
	if trimmed == "" {
		return "", fmt.Errorf("source path cannot be empty")
	}
	if strings.HasPrefix(trimmed, "/") {
		return "", fmt.Errorf("source path %q must be relative", trimmed)
	}
	if strings.ContainsRune(trimmed, 0) {
		return "", fmt.Errorf("source path %q contains invalid bytes", trimmed)
	}

	cleaned := path.Clean(trimmed)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "/../") {
		return "", fmt.Errorf("source path %q escapes the suite root", trimmed)
	}
	if cleaned != trimmed {
		return "", fmt.Errorf("source path %q must be normalized", trimmed)
	}

	for _, segment := range strings.Split(cleaned, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", fmt.Errorf("source path %q contains an invalid segment", trimmed)
		}
		if strings.HasPrefix(segment, ".") {
			return "", fmt.Errorf("source path %q contains a hidden segment", trimmed)
		}
	}

	extension := strings.ToLower(filepath.Ext(cleaned))
	if _, blocked := blockedSuiteExtensions[extension]; blocked {
		return "", fmt.Errorf("source path %q uses a blocked file type", trimmed)
	}
	if extension != "" {
		if _, ok := allowedSuiteExtensions[extension]; !ok {
			return "", fmt.Errorf("source path %q uses an unsupported file type", trimmed)
		}
	}

	return cleaned, nil
}

func validateSuiteSourceContent(filePath, content string) error {
	if len(content) > maxSuiteSourceBytes {
		return fmt.Errorf("source file %q exceeds the %d byte limit", filePath, maxSuiteSourceBytes)
	}
	if strings.ContainsRune(content, 0) {
		return fmt.Errorf("source file %q appears to contain binary content", filePath)
	}
	if !utf8.ValidString(content) {
		return fmt.Errorf("source file %q is not valid UTF-8 text", filePath)
	}
	return nil
}
