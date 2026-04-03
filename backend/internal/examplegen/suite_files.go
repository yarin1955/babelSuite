package examplegen

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/babelsuite/babelsuite/internal/suites"
	"gopkg.in/yaml.v3"
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

			files = append(files, suites.SourceFile{
				Path:     path,
				Language: detectSourceLanguage(path),
				Content:  generatedSourceContent(suite, path),
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

func detectSourceLanguage(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		return "yaml"
	case ".json":
		return "json"
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

func renderGatewaySource(suite suites.Definition, path string) string {
	if content, ok := suites.GeneratedSourceContent(suite, path); ok {
		return content
	}
	return fmt.Sprintf("# %s\n# Gateway preview is not available for %s yet.\n", suite.Title, path)
}

func renderProfileSource(suite suites.Definition, fileName string) string {
	label := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	description := "Runtime overrides for local execution."
	defaultProfile := false
	for _, profile := range suite.Profiles {
		if profile.FileName == fileName {
			label = profile.Label
			description = profile.Description
			defaultProfile = profile.Default
			break
		}
	}

	moduleLines := make([]string, 0, len(suite.Modules))
	for _, module := range suite.Modules {
		moduleLines = append(moduleLines, fmt.Sprintf("    - %s", module))
	}
	if len(moduleLines) == 0 {
		moduleLines = append(moduleLines, "    - runtime")
	}

	return strings.Join([]string{
		fmt.Sprintf("name: %s", label),
		fmt.Sprintf("description: %s", description),
		fmt.Sprintf("default: %t", defaultProfile),
		"runtime:",
		fmt.Sprintf("  suite: %s", suite.ID),
		fmt.Sprintf("  repository: %s", suite.Repository),
		fmt.Sprintf("  profileFile: %s", fileName),
		"modules:",
		strings.Join(moduleLines, "\n"),
		"observability:",
		"  logs: structured",
		"  traces: enabled",
		"  metrics: enabled",
	}, "\n") + "\n"
}

func renderOpenAPISource(suite suites.Definition) string {
	builder := &strings.Builder{}
	builder.WriteString("openapi: 3.1.0\n")
	builder.WriteString("info:\n")
	builder.WriteString(fmt.Sprintf("  title: %s\n", suite.Title))
	builder.WriteString(fmt.Sprintf("  version: %s\n", suite.Version))
	builder.WriteString("servers:\n")
	builder.WriteString(fmt.Sprintf("  - url: https://%s.mock.internal\n", suite.ID))
	builder.WriteString("paths:\n")

	wrotePath := false
	for _, surface := range suite.APISurfaces {
		for _, operation := range surface.Operations {
			method := strings.ToLower(strings.TrimSpace(operation.Method))
			if method == "rpc" || !strings.HasPrefix(operation.Name, "/") {
				continue
			}
			builder.WriteString(fmt.Sprintf("  %s:\n", operation.Name))
			builder.WriteString(fmt.Sprintf("    %s:\n", method))
			builder.WriteString(fmt.Sprintf("      operationId: %s\n", sanitizeIdentifier(operation.ID)))
			builder.WriteString(fmt.Sprintf("      summary: %s\n", operation.Summary))
			builder.WriteString("      responses:\n")
			builder.WriteString(`        "200":` + "\n")
			builder.WriteString("          description: Successful mock response\n")
			wrotePath = true
		}
	}

	if !wrotePath {
		builder.WriteString("  /healthz:\n")
		builder.WriteString("    get:\n")
		builder.WriteString("      operationId: healthz\n")
		builder.WriteString("      summary: Health probe for the suite API.\n")
		builder.WriteString("      responses:\n")
		builder.WriteString(`        "200":` + "\n")
		builder.WriteString("          description: Healthy\n")
	}

	return builder.String()
}

func renderProtoSource(suite suites.Definition, fileName string) string {
	serviceName := sanitizeIdentifier(strings.TrimSuffix(fileName, filepath.Ext(fileName)))
	if serviceName == "" {
		serviceName = sanitizeIdentifier(suite.ID)
	}

	rpcLines := make([]string, 0)
	for _, surface := range suite.APISurfaces {
		for _, operation := range surface.Operations {
			if !strings.EqualFold(operation.Method, "rpc") {
				continue
			}
			name := operation.Name
			if slash := strings.LastIndex(name, "/"); slash >= 0 {
				name = name[slash+1:]
			}
			name = sanitizeIdentifier(name)
			rpcLines = append(rpcLines, fmt.Sprintf("  rpc %s (%sRequest) returns (%sResponse);", name, name, name))
		}
	}
	if len(rpcLines) == 0 {
		rpcLines = append(rpcLines, "  rpc Ping (PingRequest) returns (PingResponse);")
	}

	return strings.Join([]string{
		`syntax = "proto3";`,
		"",
		fmt.Sprintf("package %s.v1;", strings.ReplaceAll(sanitizeIdentifier(suite.ID), "-", "")),
		"",
		fmt.Sprintf("service %sService {", serviceName),
		strings.Join(rpcLines, "\n"),
		"}",
		"",
		"message PingRequest {}",
		"",
		"message PingResponse {",
		"  string status = 1;",
		"}",
	}, "\n") + "\n"
}

func renderWSDLSource(suite suites.Definition, fileName string) string {
	serviceName := sanitizeIdentifier(strings.TrimSuffix(fileName, filepath.Ext(fileName)))
	if serviceName == "" {
		serviceName = sanitizeIdentifier(suite.ID)
	}

	type soapAction struct {
		name string
	}

	actions := make([]soapAction, 0)
	seen := make(map[string]struct{})
	location := "https://" + suite.ID + ".mock.internal"
	for _, surface := range suite.APISurfaces {
		if !strings.EqualFold(surface.Protocol, "SOAP") {
			continue
		}
		for _, operation := range surface.Operations {
			if host := strings.TrimRight(strings.TrimSpace(surface.MockHost), "/"); host != "" && strings.HasPrefix(strings.TrimSpace(operation.Name), "/") {
				location = host + strings.TrimSpace(operation.Name)
			}
			if len(operation.Exchanges) == 0 {
				name := sanitizeIdentifier(operation.ID)
				if name == "" {
					name = "Invoke"
				}
				if _, ok := seen[name]; ok {
					continue
				}
				seen[name] = struct{}{}
				actions = append(actions, soapAction{name: name})
				continue
			}
			for _, exchange := range operation.Exchanges {
				name := sanitizeIdentifier(exchange.Name)
				if name == "" {
					name = sanitizeIdentifier(operation.ID)
				}
				if name == "" {
					name = "Invoke"
				}
				if _, ok := seen[name]; ok {
					continue
				}
				seen[name] = struct{}{}
				actions = append(actions, soapAction{name: name})
			}
		}
	}
	if len(actions) == 0 {
		actions = append(actions, soapAction{name: "Invoke"})
	}

	targetNamespace := "urn:babelsuite:" + strings.ReplaceAll(strings.ToLower(suite.ID), "_", "-")
	portTypeName := serviceName + "PortType"
	bindingName := serviceName + "Binding"
	serviceBlockName := serviceName + "Service"

	lines := []string{
		`<?xml version="1.0" encoding="UTF-8"?>`,
		`<definitions xmlns="http://schemas.xmlsoap.org/wsdl/" xmlns:soap="http://schemas.xmlsoap.org/wsdl/soap/" xmlns:tns="` + targetNamespace + `" xmlns:xsd="http://www.w3.org/2001/XMLSchema" name="` + serviceBlockName + `" targetNamespace="` + targetNamespace + `">`,
		`  <types>`,
		`    <xsd:schema targetNamespace="` + targetNamespace + `">`,
	}
	for _, action := range actions {
		lines = append(lines,
			`      <xsd:element name="`+action.name+`Request" type="xsd:string"/>`,
			`      <xsd:element name="`+action.name+`Response" type="xsd:string"/>`,
		)
	}
	lines = append(lines,
		`    </xsd:schema>`,
		`  </types>`,
		"",
	)
	for _, action := range actions {
		lines = append(lines,
			`  <message name="`+action.name+`Input">`,
			`    <part name="parameters" element="tns:`+action.name+`Request"/>`,
			`  </message>`,
			`  <message name="`+action.name+`Output">`,
			`    <part name="parameters" element="tns:`+action.name+`Response"/>`,
			`  </message>`,
			"",
		)
	}
	lines = append(lines, `  <portType name="`+portTypeName+`">`)
	for _, action := range actions {
		lines = append(lines,
			`    <operation name="`+action.name+`">`,
			`      <input message="tns:`+action.name+`Input"/>`,
			`      <output message="tns:`+action.name+`Output"/>`,
			`    </operation>`,
		)
	}
	lines = append(lines,
		`  </portType>`,
		`  <binding name="`+bindingName+`" type="tns:`+portTypeName+`">`,
		`    <soap:binding transport="http://schemas.xmlsoap.org/soap/http" style="document"/>`,
	)
	for _, action := range actions {
		lines = append(lines,
			`    <operation name="`+action.name+`">`,
			`      <soap:operation soapAction="urn:`+action.name+`"/>`,
			`      <input><soap:body use="literal"/></input>`,
			`      <output><soap:body use="literal"/></output>`,
			`    </operation>`,
		)
	}
	lines = append(lines,
		`  </binding>`,
		`  <service name="`+serviceBlockName+`">`,
		`    <port name="`+serviceName+`Port" binding="tns:`+bindingName+`">`,
		`      <soap:address location="`+location+`"/>`,
		`    </port>`,
		`  </service>`,
		`</definitions>`,
	)

	return strings.Join(lines, "\n") + "\n"
}

func renderMockSource(suite suites.Definition, path string) string {
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
		return formatJSON(map[string]any{
			"artifact": path,
			"suite":    suite.ID,
			"message":  "No seeded exchanges matched this mock file, so BabelSuite generated a placeholder preview.",
		})
	}

	if !strings.HasSuffix(strings.ToLower(path), ".json") {
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
		return formatJSON(payload)
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
	return formatJSON(document)
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

func renderScriptSource(suite suites.Definition, path string) string {
	base := filepath.Base(path)
	switch detectSourceLanguage(path) {
	case "bash":
		return strings.Join([]string{
			"#!/usr/bin/env bash",
			"set -euo pipefail",
			"",
			fmt.Sprintf("echo \"bootstrapping %s for %s\"", base, suite.Title),
			fmt.Sprintf("echo \"resolved modules: %s\"", strings.Join(suite.Modules, ", ")),
		}, "\n") + "\n"
	case "python":
		return strings.Join([]string{
			"from pathlib import Path",
			"",
			fmt.Sprintf("SUITE_ID = %q", suite.ID),
			fmt.Sprintf("SCRIPT_NAME = %q", base),
			"",
			"def main() -> None:",
			"    print(f\"running {SCRIPT_NAME} for {SUITE_ID}\")",
			"    print(Path('.').resolve())",
			"",
			`if __name__ == "__main__":`,
			"    main()",
		}, "\n") + "\n"
	default:
		return strings.Join([]string{
			fmt.Sprintf("const suiteId = %q", suite.ID),
			fmt.Sprintf("const scriptName = %q", base),
			"",
			"console.log(`running ${scriptName} for ${suiteId}`)",
		}, "\n") + "\n"
	}
}

func renderScenarioSource(suite suites.Definition, path string) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if detectSourceLanguage(path) == "typescript" {
		return strings.Join([]string{
			`import { test, expect } from "@playwright/test"`,
			"",
			fmt.Sprintf("test(%q, async ({ page }) => {", strings.ReplaceAll(base, "_", " ")),
			"  await page.goto('/')",
			fmt.Sprintf("  await expect(page.getByText(%q)).toBeVisible()", suite.Title),
			"})",
		}, "\n") + "\n"
	}

	return strings.Join([]string{
		"def test_smoke() -> None:",
		fmt.Sprintf("    assert %q", suite.Title),
		fmt.Sprintf("    assert %q in %q", suite.ID, suite.Repository),
	}, "\n") + "\n"
}

func renderLoadSource(suite suites.Definition, path string) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	switch detectSourceLanguage(path) {
	case "python":
		return strings.Join([]string{
			"from pathlib import Path",
			"",
			fmt.Sprintf("LOAD_ASSET = %q", base),
			"",
			"def main() -> None:",
			fmt.Sprintf("    print(%q)", "running generic load asset"),
			"    print(Path('.').resolve())",
			"",
			`if __name__ == "__main__":`,
			"    main()",
		}, "\n") + "\n"
	case "xml":
		return strings.Join([]string{
			`<?xml version="1.0" encoding="UTF-8"?>`,
			"<loadPlan>",
			fmt.Sprintf("  <suite>%s</suite>", suite.ID),
			fmt.Sprintf("  <asset>%s</asset>", base),
			"</loadPlan>",
		}, "\n") + "\n"
	case "yaml":
		return strings.Join([]string{
			"targets:",
			"  p95LatencyMs: 450",
			"  errorRatePercent: 1",
			"  rampUsers: 120",
			"  steadyStateSeconds: 180",
			fmt.Sprintf("suite: %s", suite.ID),
		}, "\n") + "\n"
	default:
		return strings.Join([]string{
			fmt.Sprintf("# load asset for %s", suite.Title),
			fmt.Sprintf("name: %s", base),
		}, "\n") + "\n"
	}
}

func renderFixtureSource(suite suites.Definition, path string) string {
	base := strings.ToLower(filepath.Base(path))
	switch {
	case strings.HasSuffix(base, ".csv"):
		return "id,name,status\nm-117,Core Merchant,active\nm-441,Risky Merchant,review\n"
	case strings.HasSuffix(base, ".ndjson"):
		return strings.Join([]string{
			`{"vehicleId":"vh-11","speed":0,"battery":76}`,
			`{"vehicleId":"vh-12","speed":31,"battery":68}`,
		}, "\n") + "\n"
	case strings.HasSuffix(base, ".yaml"), strings.HasSuffix(base, ".yml"):
		return strings.Join([]string{
			"realm: example",
			"issuer: https://issuer.demo.test",
			"seedUsers:",
			"  - admin@babelsuite.test",
		}, "\n") + "\n"
	default:
		return formatJSON(defaultFixturePayload(suite, base))
	}
}

func renderPolicySource(suite suites.Definition, path string) string {
	policyName := sanitizeIdentifier(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	return strings.Join([]string{
		fmt.Sprintf("package babelsuite.%s", strings.ToLower(policyName)),
		"",
		`default allow := false`,
		"",
		"allow if {",
		fmt.Sprintf("  input.suite == %q", suite.ID),
		"  count(input.modules) >= 1",
		"}",
	}, "\n") + "\n"
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

func inferHeaderSchema(headers []suites.Header) map[string]any {
	if len(headers) == 0 {
		return map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	}

	properties := make(map[string]any, len(headers))
	required := make([]string, 0, len(headers))
	for _, header := range headers {
		name := strings.TrimSpace(header.Name)
		if name == "" {
			continue
		}
		required = append(required, name)
		properties[name] = inferJSONSchemaValue(header.Value)
	}

	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		sort.Strings(required)
		schema["required"] = required
	}
	return schema
}

func inferBodySchema(body string) any {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return map[string]any{"type": "null"}
	}

	var parsed any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
		return inferJSONSchemaValue(parsed)
	}

	if strings.HasPrefix(trimmed, "<") {
		schema := map[string]any{
			"type":   "string",
			"format": "xml",
		}
		if strings.Contains(trimmed, "{{") {
			schema["x-babel-template"] = trimmed
		} else {
			schema["example"] = trimmed
		}
		return schema
	}

	schema := map[string]any{
		"type": "string",
	}
	if strings.Contains(trimmed, "{{") {
		schema["x-babel-template"] = trimmed
	} else {
		schema["example"] = trimmed
	}
	return schema
}

func inferJSONSchemaValue(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		properties := make(map[string]any, len(typed))
		for key, nested := range typed {
			properties[key] = inferJSONSchemaValue(nested)
		}
		return map[string]any{
			"type":       "object",
			"properties": properties,
		}
	case []any:
		schema := map[string]any{"type": "array"}
		if len(typed) > 0 {
			schema["items"] = inferJSONSchemaValue(typed[0])
		}
		return schema
	case string:
		schema := map[string]any{"type": "string"}
		if strings.Contains(typed, "{{") {
			schema["x-babel-template"] = typed
		} else {
			schema["example"] = typed
		}
		return schema
	case bool:
		return map[string]any{"type": "boolean", "example": typed}
	case float64:
		if typed == float64(int64(typed)) {
			return map[string]any{"type": "integer", "example": int64(typed)}
		}
		return map[string]any{"type": "number", "example": typed}
	case nil:
		return map[string]any{"type": "null"}
	default:
		return map[string]any{"type": "string", "example": fmt.Sprint(typed)}
	}
}

func defaultFixturePayload(suite suites.Definition, base string) any {
	switch {
	case strings.Contains(base, "products"):
		return []map[string]any{
			{"sku": "sku_1001", "name": "Starter Keyboard", "price": 4900},
			{"sku": "sku_2024", "name": "Launch Headset", "price": 12900},
		}
	case strings.Contains(base, "users"), strings.Contains(base, "claims"):
		return []map[string]any{
			{"email": "admin@babelsuite.test", "role": "admin"},
			{"email": "shopper@demo.test", "role": "demo"},
		}
	case strings.Contains(base, "cards"):
		return []map[string]any{
			{"token": "tok_visa", "brand": "visa", "country": "US"},
			{"token": "tok_risky", "brand": "mastercard", "country": "GB"},
		}
	case strings.Contains(base, "vehicles"):
		return []map[string]any{
			{"vehicleId": "vh-11", "state": "idle"},
			{"vehicleId": "vh-12", "state": "charging"},
		}
	default:
		return map[string]any{
			"suite":   suite.ID,
			"fixture": base,
			"modules": suite.Modules,
		}
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

func formatJSON(value any) string {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "{}\n"
	}
	return string(body) + "\n"
}
