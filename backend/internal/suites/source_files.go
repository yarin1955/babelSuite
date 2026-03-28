package suites

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

func hydrateSuites(input map[string]Definition) map[string]Definition {
	output := make(map[string]Definition, len(input))
	for id, suite := range input {
		suite.SourceFiles = buildSourceFiles(suite)
		output[id] = suite
	}
	return output
}

func buildSourceFiles(suite Definition) []SourceFile {
	files := make([]SourceFile, 0)
	seen := make(map[string]struct{})

	for _, folder := range suite.Folders {
		for _, file := range folder.Files {
			path := normalizeSourcePath(folder.Name, file)
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}

			content := generatedSourceContent(suite, path)
			if loaded, ok := readExampleSourceFile(suite.ID, path); ok {
				content = loaded
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
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", false
	}

	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", ".."))
	target := filepath.Join(repoRoot, "examples", "oci-suites", suiteID, filepath.FromSlash(path))
	content, err := os.ReadFile(target)
	if err != nil {
		return "", false
	}
	return string(content), true
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

func generatedSourceContent(suite Definition, path string) string {
	switch {
	case strings.HasPrefix(path, "profiles/"):
		return renderProfileSource(suite, filepath.Base(path))
	case strings.HasPrefix(path, "api/openapi/"):
		return renderOpenAPISource(suite)
	case strings.HasPrefix(path, "api/proto/"):
		return renderProtoSource(suite, filepath.Base(path))
	case strings.HasPrefix(path, "mock/"):
		return renderMockSource(suite, path)
	case strings.HasPrefix(path, "scripts/"):
		return renderScriptSource(suite, path)
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

func renderProfileSource(suite Definition, fileName string) string {
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

func renderOpenAPISource(suite Definition) string {
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

func renderProtoSource(suite Definition, fileName string) string {
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

func renderMockSource(suite Definition, path string) string {
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

	payload := make(map[string]any, len(exchanges))
	for _, exchange := range exchanges {
		var body any
		if err := json.Unmarshal([]byte(strings.TrimSpace(exchange.ResponseBody)), &body); err != nil {
			body = map[string]any{
				"dispatchCriteria": exchange.DispatchCriteria,
				"responseBody":     exchange.ResponseBody,
			}
		}
		payload[exchange.Name] = body
	}

	return formatJSON(payload)
}

func renderScriptSource(suite Definition, path string) string {
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

func renderScenarioSource(suite Definition, path string) string {
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

func renderFixtureSource(suite Definition, path string) string {
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

func renderPolicySource(suite Definition, path string) string {
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

func exchangesForSource(suite Definition, path string) []ExchangeExample {
	normalized := strings.TrimPrefix(strings.TrimSpace(path), "mock/")
	exchanges := make([]ExchangeExample, 0)
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

func defaultFixturePayload(suite Definition, base string) any {
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
