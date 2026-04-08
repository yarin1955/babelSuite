package suites

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/babelsuite/babelsuite/internal/apisix"
	"github.com/babelsuite/babelsuite/internal/examplefs"
)

type sourceFileLoader func(suiteID, path string) (string, bool)

func buildSourceFiles(suite Definition, loader sourceFileLoader) []SourceFile {
	files := make([]SourceFile, 0)
	seen := make(map[string]struct{})
	appendFile := func(path string) {
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}

		content, generated := explicitSuiteSourceContent(suite.SeedSources, path)
		if !generated {
			content, generated = GeneratedSourceContent(suite, path)
		}
		if !generated {
			content = missingSourceContent(suite, path)
		}
		if !generated && loader != nil {
			if loaded, ok := loader(suite.ID, path); ok {
				content = loaded
			}
		}

		files = append(files, SourceFile{
			Path:     path,
			Language: detectSourceLanguage(path),
			Content:  content,
		})
	}

	for _, file := range suite.SeedSources {
		path := strings.Trim(strings.TrimSpace(file.Path), "/")
		if path == "" || strings.Contains(path, "/") {
			continue
		}
		appendFile(path)
	}

	for _, folder := range suite.Folders {
		for _, file := range folder.Files {
			path := normalizeSourcePath(folder.Name, file)
			appendFile(path)
		}
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files
}

func explicitSuiteSourceContent(files []SourceFile, path string) (string, bool) {
	for _, file := range files {
		if strings.TrimSpace(file.Path) == strings.TrimSpace(path) {
			return file.Content, true
		}
	}
	return "", false
}

func normalizeSourcePath(folderName, file string) string {
	return strings.Trim(strings.TrimSpace(folderName)+"/"+strings.Trim(strings.TrimSpace(file), "/"), "/")
}

func GeneratedSourceContent(suite Definition, path string) (string, bool) {
	normalized := strings.Trim(strings.TrimSpace(path), "/")
	if normalized != "gateway/apisix.yaml" || len(suite.APISurfaces) == 0 {
		return "", false
	}
	return apisix.RenderStandaloneConfig(apisixSuiteConfig(suite)), true
}

func readExampleSourceFile(suiteID, path string) (string, bool) {
	target := examplefs.SuiteFilePath(suiteID, path)
	content, err := os.ReadFile(target)
	if err != nil {
		return "", false
	}
	return string(content), true
}

func missingSourceContent(suite Definition, path string) string {
	return fmt.Sprintf(
		"# Missing example source for %s\n# Expected file: %s\n# Configure %s to point at the shared examples folder.\n",
		path,
		examplefs.SuiteFilePath(suite.ID, path),
		examplefs.RootEnvVar,
	)
}

func detectSourceLanguage(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		return "yaml"
	case ".json":
		return "json"
	case ".cue":
		return "cue"
	case ".xml", ".wsdl", ".xsd":
		return "xml"
	case ".proto":
		return "protobuf"
	case ".rego":
		return "rego"
	case ".py":
		return "python"
	case ".sh":
		return "bash"
	case ".ts":
		return "typescript"
	case ".csv":
		return "csv"
	case ".ndjson":
		return "json"
	default:
		return "text"
	}
}

func sanitizeIdentifier(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "Example"
	}

	replacer := strings.NewReplacer("/", "_", "-", "_", ".", "_", " ", "_")
	value = replacer.Replace(value)
	return strings.Trim(value, "_")
}

func apisixSuiteConfig(suite Definition) apisix.SuiteConfig {
	output := apisix.SuiteConfig{
		ID:          suite.ID,
		APISurfaces: make([]apisix.SurfaceConfig, 0, len(suite.APISurfaces)),
	}
	for _, surface := range suite.APISurfaces {
		convertedSurface := apisix.SurfaceConfig{
			ID:         surface.ID,
			Protocol:   surface.Protocol,
			MockHost:   surface.MockHost,
			Operations: make([]apisix.OperationConfig, 0, len(surface.Operations)),
		}
		for _, operation := range surface.Operations {
			convertedSurface.Operations = append(convertedSurface.Operations, apisix.OperationConfig{
				ID:              operation.ID,
				Method:          operation.Method,
				Name:            operation.Name,
				Summary:         operation.Summary,
				ContractPath:    operation.ContractPath,
				ContractContent: contractSourceContent(suite, operation.ContractPath),
				MockURL:         operation.MockURL,
				MockMetadata: apisix.OperationMetadataConfig{
					Adapter:         operation.MockMetadata.Adapter,
					DispatcherRules: operation.MockMetadata.DispatcherRules,
					ResolverURL:     operation.MockMetadata.ResolverURL,
					RuntimeURL:      operation.MockMetadata.RuntimeURL,
				},
			})
		}
		output.APISurfaces = append(output.APISurfaces, convertedSurface)
	}
	return output
}

func contractSourceContent(suite Definition, contractPath string) string {
	path := strings.TrimSpace(contractFilePath(contractPath))
	if path == "" || !strings.EqualFold(filepath.Ext(path), ".proto") {
		return ""
	}

	if content, ok := explicitSuiteSourceContent(suite.SeedSources, path); ok {
		return content
	}
	if content, ok := readExampleSourceFile(suite.ID, path); ok {
		return content
	}
	return ""
}

func contractFilePath(contractPath string) string {
	base, _, _ := strings.Cut(strings.TrimSpace(contractPath), "#")
	return strings.Trim(base, "/")
}
