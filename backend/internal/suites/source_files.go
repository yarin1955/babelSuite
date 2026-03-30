package suites

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/babelsuite/babelsuite/internal/examplefs"
)

type sourceFileLoader func(suiteID, path string) (string, bool)

func hydrateSuites(input map[string]Definition) map[string]Definition {
	output := make(map[string]Definition, len(input))
	for id, suite := range input {
		suite = normalizeDefinition(suite)
		suite.SourceFiles = buildSourceFiles(suite, readExampleSourceFile)
		output[id] = suite
	}
	return output
}

func normalizeDefinition(suite Definition) Definition {
	for surfaceIndex, surface := range suite.APISurfaces {
		for operationIndex, operation := range surface.Operations {
			metadata := operation.MockMetadata
			if metadata.Adapter == "" {
				metadata.Adapter = defaultMockAdapter(surface.Protocol)
			}
			if metadata.MetadataPath == "" && strings.HasPrefix(strings.TrimSpace(operation.MockPath), "mock/") {
				metadata.MetadataPath = metadataPathForMockPath(operation.MockPath)
			}
			if metadata.RuntimeURL == "" {
				metadata.RuntimeURL = runtimeURLForOperation(suite.ID, surface, operation)
			}
			operation.MockMetadata = metadata
			suite.APISurfaces[surfaceIndex].Operations[operationIndex] = operation
			suite = ensureMockMetadataFile(suite, metadata.MetadataPath)
		}
	}
	return suite
}

func defaultMockAdapter(protocol string) string {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "grpc":
		return "grpc"
	case "async":
		return "async"
	default:
		return "rest"
	}
}

func metadataPathForMockPath(mockPath string) string {
	trimmed := strings.TrimSpace(strings.TrimPrefix(mockPath, "mock/"))
	ext := filepath.Ext(trimmed)
	base := strings.TrimSuffix(trimmed, ext)
	return normalizeSourcePath("mock", base+".metadata.yaml")
}

func ensureMockMetadataFile(suite Definition, metadataPath string) Definition {
	if metadataPath == "" {
		return suite
	}

	fileName := strings.TrimPrefix(strings.TrimSpace(metadataPath), "mock/")
	for folderIndex, folder := range suite.Folders {
		if folder.Name != "mock" {
			continue
		}
		for _, existing := range folder.Files {
			if strings.TrimSpace(existing) == fileName {
				return suite
			}
		}
		suite.Folders[folderIndex].Files = append(suite.Folders[folderIndex].Files, fileName)
		sort.Strings(suite.Folders[folderIndex].Files)
		return suite
	}
	return suite
}

func runtimeURLForOperation(suiteID string, surface APISurface, operation APIOperation) string {
	base := "/mocks/" + defaultMockAdapter(surface.Protocol) + "/" + suiteID + "/" + surface.ID
	switch defaultMockAdapter(surface.Protocol) {
	case "grpc", "async":
		return base + "/" + operation.ID
	default:
		path := operation.Name
		if !strings.HasPrefix(path, "/") {
			path = "/" + sanitizeIdentifier(path)
		}
		return base + fillPathParameters(path, operation)
	}
}

func fillPathParameters(path string, operation APIOperation) string {
	if !strings.Contains(path, "{") {
		return path + runtimeQuerySuffix(operation)
	}
	if len(operation.Exchanges) > 0 {
		for _, cond := range operation.Exchanges[0].When {
			if cond.From == "path" {
				path = strings.ReplaceAll(path, "{"+cond.Param+"}", cond.Value)
			}
		}
	}
	return path + runtimeQuerySuffix(operation)
}

func runtimeQuerySuffix(operation APIOperation) string {
	if len(operation.Exchanges) == 0 {
		return ""
	}
	query := make([]string, 0)
	for _, cond := range operation.Exchanges[0].When {
		if cond.From == "query" {
			query = append(query, cond.Param+"="+cond.Value)
		}
	}
	sort.Strings(query)
	if len(query) == 0 {
		return ""
	}
	return "?" + strings.Join(query, "&")
}

func buildSourceFiles(suite Definition, loader sourceFileLoader) []SourceFile {
	files := make([]SourceFile, 0)
	seen := make(map[string]struct{})

	for _, folder := range suite.Folders {
		for _, file := range folder.Files {
			path := normalizeSourcePath(folder.Name, file)
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}

			content := missingSourceContent(suite, path)
			if loader != nil {
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
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files
}

func normalizeSourcePath(folderName, file string) string {
	return strings.Trim(strings.TrimSpace(folderName)+"/"+strings.Trim(strings.TrimSpace(file), "/"), "/")
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
	case ".xml":
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
