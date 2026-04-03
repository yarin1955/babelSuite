package examplegen

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/babelsuite/babelsuite/internal/suites"
	"gopkg.in/yaml.v3"
)

func renderMockSource(suite suites.Definition, path string) string {
	if content, ok := explicitSourceContent(suite.SeedSources, path); ok {
		return content
	}

	if strings.HasSuffix(strings.ToLower(path), ".xml") {
		return strings.Join([]string{
			`<?xml version="1.0" encoding="UTF-8"?>`,
			"<mockResponse>",
			fmt.Sprintf("  <suite>%s</suite>", suite.ID),
			fmt.Sprintf("  <artifact>%s</artifact>", path),
			"  <status>ok</status>",
			"</mockResponse>",
		}, "\n")
	}

	exchanges := exchangesForSource(suite, path)
	if len(exchanges) == 0 {
		return formatMockSchemaDocument(path, map[string]any{
			"artifact": path,
			"suite":    suite.ID,
			"message":  "No seeded exchanges matched this mock file, so BabelSuite generated a placeholder preview.",
		})
	}

	if !isSchemaBackedMockDocument(path) {
		payload := make(map[string]any, len(exchanges))
		for _, exchange := range exchanges {
			var body any
			if err := json.Unmarshal([]byte(strings.TrimSpace(exchange.ResponseBody)), &body); err != nil {
				body = map[string]any{
					"when":         exchange.When,
					"responseBody": exchange.ResponseBody,
				}
			}
			payload[exchange.Name] = body
		}
		return formatMockSchemaDocument(path, payload)
	}

	operation, found := operationForMockPath(suite, path)
	exampleSchemas := make(map[string]any, len(exchanges))
	for _, exchange := range exchanges {
		exampleSchemas[exchange.Name] = map[string]any{
			"dispatch": exchange.When,
			"requestSchema": map[string]any{
				"headers": inferHeaderSchema(exchange.RequestHeaders),
				"body":    inferBodySchema(exchange.RequestBody),
			},
			"responseSchema": map[string]any{
				"status":    exchange.ResponseStatus,
				"mediaType": exchange.ResponseMediaType,
				"headers":   inferHeaderSchema(exchange.ResponseHeaders),
				"body":      inferBodySchema(exchange.ResponseBody),
			},
		}
	}

	document := map[string]any{
		"$schema":  "https://schemas.babelsuite.dev/mock-exchange-source-v1.json",
		"suite":    suite.ID,
		"artifact": path,
		"examples": exampleSchemas,
	}
	if found {
		document["operationId"] = operation.ID
		document["adapter"] = operation.MockMetadata.Adapter
		document["dispatcher"] = operation.Dispatcher
		document["contractPath"] = operation.ContractPath
		document["resolverUrl"] = operation.MockMetadata.ResolverURL
		document["runtimeUrl"] = operation.MockMetadata.RuntimeURL
	}
	return formatMockSchemaDocument(path, document)
}

func isSchemaBackedMockDocument(path string) bool {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(path))) {
	case ".json", ".cue":
		return true
	default:
		return false
	}
}

func renderMockMetadataSource(suite suites.Definition, path string) string {
	operation, ok := operationForMetadataPath(suite, path)
	if !ok {
		return strings.Join([]string{
			"apiVersion: mocks.babelsuite.io/v1alpha1",
			"kind: MockOperation",
			"metadata:",
			fmt.Sprintf("  suite: %s", suite.ID),
			fmt.Sprintf("  path: %s", path),
			"spec:",
			"  message: no mock operation metadata matched this file",
		}, "\n") + "\n"
	}

	document := map[string]any{
		"apiVersion": "mocks.babelsuite.io/v1alpha1",
		"kind":       "MockOperation",
		"metadata": map[string]any{
			"suite":          suite.ID,
			"operationId":    operation.ID,
			"metadataPath":   operation.MockMetadata.MetadataPath,
			"sourceArtifact": operation.MockPath,
		},
		"spec": map[string]any{
			"adapter":              operation.MockMetadata.Adapter,
			"delayMillis":          operation.MockMetadata.DelayMillis,
			"resolverUrl":          operation.MockMetadata.ResolverURL,
			"runtimeUrl":           operation.MockMetadata.RuntimeURL,
			"parameterConstraints": operation.MockMetadata.ParameterConstraints,
			"fallback":             operation.MockMetadata.Fallback,
			"state":                operation.MockMetadata.State,
		},
	}

	body, err := yaml.Marshal(document)
	if err != nil {
		return "apiVersion: mocks.babelsuite.io/v1alpha1\nkind: MockOperation\n"
	}
	return string(body)
}

func exchangesForSource(suite suites.Definition, path string) []suites.ExchangeExample {
	normalized := strings.TrimPrefix(strings.TrimSpace(path), "mock/")
	exchanges := make([]suites.ExchangeExample, 0)
	for _, surface := range suite.APISurfaces {
		for _, operation := range surface.Operations {
			operationPath := strings.TrimPrefix(strings.TrimSpace(operation.MockPath), "mock/")
			if operationPath != normalized {
				continue
			}
			exchanges = append(exchanges, operation.Exchanges...)
		}
	}
	return exchanges
}

func operationForMetadataPath(suite suites.Definition, path string) (suites.APIOperation, bool) {
	normalized := strings.TrimSpace(path)
	for _, surface := range suite.APISurfaces {
		for _, operation := range surface.Operations {
			if strings.TrimSpace(operation.MockMetadata.MetadataPath) == normalized {
				return operation, true
			}
		}
	}
	return suites.APIOperation{}, false
}

func operationForMockPath(suite suites.Definition, path string) (suites.APIOperation, bool) {
	normalized := strings.TrimSpace(path)
	for _, surface := range suite.APISurfaces {
		for _, operation := range surface.Operations {
			if strings.TrimSpace(operation.MockPath) == normalized {
				return operation, true
			}
		}
	}
	return suites.APIOperation{}, false
}
