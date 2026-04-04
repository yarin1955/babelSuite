package suites

import (
	"path/filepath"
	"sort"
	"strings"
)

func hydrateSuites(input map[string]Definition) map[string]Definition {
	output := make(map[string]Definition, len(input))
	for id, suite := range input {
		suite = normalizeDefinition(suite)
		suite.SourceFiles = buildSourceFiles(suite, readExampleSourceFile)
		suite.SeedSources = cloneSourceFiles(suite.SourceFiles)
		output[id] = suite
	}
	return output
}

func normalizeDefinition(suite Definition) Definition {
	suite = normalizeMockSchemaArtifacts(suite)
	for surfaceIndex, surface := range suite.APISurfaces {
		for operationIndex, operation := range surface.Operations {
			metadata := operation.MockMetadata
			if metadata.Adapter == "" {
				metadata.Adapter = defaultMockAdapter(surface.Protocol)
			}
			if metadata.Dispatcher == "" {
				metadata.Dispatcher = defaultMockDispatcher(surface, operation)
			}
			if metadata.MetadataPath == "" && strings.HasPrefix(strings.TrimSpace(operation.MockPath), "mock/") {
				metadata.MetadataPath = metadataPathForMockPath(operation.MockPath)
			}
			if metadata.ResolverURL == "" {
				metadata.ResolverURL = resolverURLForOperation(suite.ID, surface, operation)
			}
			if metadata.RuntimeURL == "" {
				metadata.RuntimeURL = runtimeURLForOperation(suite.ID, surface, operation)
			}
			if metadata.DispatcherRules == "" {
				metadata.DispatcherRules = defaultDispatcherRules(suite.ID, surface, operation, metadata)
			}
			operation.MockMetadata = metadata
			if operation.Dispatcher == "" {
				operation.Dispatcher = metadata.Dispatcher
			}
			suite.APISurfaces[surfaceIndex].Operations[operationIndex] = operation
			suite = ensureMockMetadataFile(suite, metadata.MetadataPath)
		}
	}
	return ensureGatewayFolder(suite)
}

func normalizeMockSchemaArtifacts(suite Definition) Definition {
	for folderIndex := range suite.Folders {
		if suite.Folders[folderIndex].Name != "mock" {
			continue
		}
		for fileIndex, file := range suite.Folders[folderIndex].Files {
			suite.Folders[folderIndex].Files[fileIndex] = strings.TrimPrefix(normalizeMockSchemaPath("mock/"+strings.Trim(file, "/")), "mock/")
		}
		sort.Strings(suite.Folders[folderIndex].Files)
	}

	for surfaceIndex, surface := range suite.APISurfaces {
		for operationIndex, operation := range surface.Operations {
			operation.MockPath = normalizeMockSchemaPath(operation.MockPath)
			for exchangeIndex, exchange := range operation.Exchanges {
				operation.Exchanges[exchangeIndex].SourceArtifact = normalizeMockSchemaReference(exchange.SourceArtifact)
			}
			suite.APISurfaces[surfaceIndex].Operations[operationIndex] = operation
		}
	}

	return suite
}

func normalizeMockSchemaPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if !strings.HasPrefix(trimmed, "mock/") {
		return trimmed
	}
	if !strings.EqualFold(filepath.Ext(trimmed), ".json") {
		return trimmed
	}
	return strings.TrimSuffix(trimmed, filepath.Ext(trimmed)) + ".cue"
}

func normalizeMockSchemaReference(path string) string {
	trimmed := strings.TrimSpace(path)
	if !strings.EqualFold(filepath.Ext(trimmed), ".json") {
		return trimmed
	}
	return strings.TrimSuffix(trimmed, filepath.Ext(trimmed)) + ".cue"
}

func defaultMockAdapter(protocol string) string {
	switch normalizeTransportName(protocol) {
	case "grpc":
		return "grpc"
	case "async", "kafka", "mqtt", "amqp", "nats", "tcp", "udp":
		return "async"
	case "soap", "graphql", "websocket", "sse", "webhook":
		return "rest"
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
