package examplegen

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/babelsuite/babelsuite/internal/suites"
)

func GeneratedSourceFiles(suite suites.Definition) []suites.SourceFile {
	files := make([]suites.SourceFile, 0)
	seen := make(map[string]struct{})

	for _, folder := range suite.Folders {
		for _, file := range folder.Files {
			path := normalizeSourcePath(folder.Name, file)
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}

			content, ok := explicitSourceContent(suite.SeedSources, path)
			if !ok {
				content = generatedSourceContent(suite, path)
			}
			files = append(files, suites.SourceFile{
				Path:     path,
				Language: detectSourceLanguage(path),
				Content:  content,
			})
		}
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files
}

func explicitSourceContent(files []suites.SourceFile, path string) (string, bool) {
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

func generatedSourceContent(suite suites.Definition, path string) string {
	switch {
	case strings.HasPrefix(path, "profiles/"):
		return renderProfileSource(suite, filepath.Base(path))
	case strings.HasPrefix(path, "api/openapi/"):
		return renderOpenAPISource(suite)
	case strings.HasPrefix(path, "api/wsdl/"):
		return renderWSDLSource(suite, filepath.Base(path))
	case strings.HasPrefix(path, "api/proto/"):
		return renderProtoSource(suite, filepath.Base(path))
	case strings.HasPrefix(path, "mock/") && strings.HasSuffix(strings.ToLower(path), ".metadata.yaml"):
		return renderMockMetadataSource(suite, path)
	case strings.HasPrefix(path, "mock/"):
		return renderMockSource(suite, path)
	case strings.HasPrefix(path, "scripts/"):
		return renderScriptSource(suite, path)
	case strings.HasPrefix(path, "load/"):
		return renderLoadSource(suite, path)
	case strings.HasPrefix(path, "gateway/"):
		return renderGatewaySource(suite, path)
	case strings.HasPrefix(path, "scenarios/"):
		return renderScenarioSource(suite, path)
	case strings.HasPrefix(path, "fixtures/"):
		return renderFixtureSource(suite, path)
	case strings.HasPrefix(path, "policies/"):
		return renderPolicySource(suite, path)
	default:
		return fmt.Sprintf("# %s\n# Source preview is not available for %s yet.\n", suite.Title, path)
	}
}
