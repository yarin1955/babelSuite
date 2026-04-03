package examplegen

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/babelsuite/babelsuite/internal/suites"
)

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
