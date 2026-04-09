package createcmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/babelsuite/babelsuite/cli/ctl/internal/support"
	"github.com/babelsuite/babelsuite/pkg/apiclient"
)

func Run(_ context.Context, rt *support.Runtime, opts support.GlobalOptions, args []string) int {
	request, showHelp, err := parseCreateRequest(args)
	if err != nil {
		rt.Fail(err)
		printHelp(rt)
		return 1
	}
	if showHelp {
		printHelp(rt)
		return 0
	}

	written, err := support.WriteSuiteFiles(request.destination, templateFiles(request.name, request.title), request.force)
	if err != nil {
		rt.Fail(err)
		return 1
	}

	if opts.Output == "json" {
		_ = support.PrintJSON(rt.Stdout, map[string]any{
			"name":        request.name,
			"title":       request.title,
			"destination": request.destination,
			"files":       written,
		})
		return 0
	}

	support.PrintKeyValues(rt.Stdout, [][2]string{
		{"Created template", request.title},
		{"Suite name", request.name},
		{"Destination", request.destination},
		{"Files written", fmt.Sprintf("%d", written)},
	})
	return 0
}

type createRequest struct {
	name        string
	title       string
	destination string
	force       bool
}

func parseCreateRequest(args []string) (createRequest, bool, error) {
	request := createRequest{}
	if len(args) == 0 {
		return request, true, nil
	}

	if args[0] == "template" || args[0] == "suite" {
		args = args[1:]
	}
	if len(args) == 0 {
		return request, true, nil
	}

	positionals := make([]string, 0, 2)
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "-h", "--help", "help":
			return request, true, nil
		case "--force":
			request.force = true
		case "--title":
			index++
			if index >= len(args) {
				return request, false, errors.New("--title requires a value")
			}
			request.title = strings.TrimSpace(args[index])
		default:
			if strings.HasPrefix(args[index], "-") {
				return request, false, fmt.Errorf("unknown create option %q", args[index])
			}
			positionals = append(positionals, args[index])
		}
	}

	if len(positionals) == 0 {
		return request, false, errors.New("create requires a template name")
	}
	if len(positionals) > 2 {
		return request, false, errors.New("create accepts at most a template name and destination")
	}

	request.name = normalizeTemplateName(positionals[0])
	if request.name == "" {
		return request, false, errors.New("template name must contain letters or numbers")
	}
	if request.title == "" {
		request.title = titleizeTemplateName(request.name)
	}
	if len(positionals) == 1 {
		request.destination = request.name
	} else {
		request.destination = positionals[1]
	}
	return request, false, nil
}

func templateFiles(name string, title string) []apiclient.SuiteSourceFile {
	return []apiclient.SuiteSourceFile{
		{Path: "metadata.yaml", Language: "yaml", Content: renderMetadataYAML(name, title)},
		{Path: "suite.star", Language: "starlark", Content: renderSuiteStar(name)},
		{Path: "profiles/local.yaml", Language: "yaml", Content: renderLocalProfile(name, title)},
		{Path: "api/openapi.yaml", Language: "yaml", Content: renderOpenAPI(title)},
		{Path: "mock/catalog/get-item.cue", Language: "cue", Content: renderMockCue()},
		{Path: "mock/catalog/get-item.metadata.yaml", Language: "yaml", Content: renderMockMetadata(name)},
		{Path: "services/README.md", Language: "markdown", Content: "# Services\n\nUse this folder for background infrastructure assets, compatibility sidecars, and service-specific support files.\n"},
		{Path: "tasks/bootstrap.sh", Language: "bash", Content: renderBootstrapScript()},
		{Path: "traffic/smoke.star", Language: "starlark", Content: renderLoadPlan()},
		{Path: "tests/http/smoke.hurl", Language: "hurl", Content: "GET {{BASE_URL}}/health\nHTTP 200\n"},
		{Path: "docs/README.md", Language: "markdown", Content: renderDocsReadme(name, title)},
		{Path: "resources/data/README.md", Language: "markdown", Content: "# Data Assets\n\nUse this folder for passive datasets, payload samples, and large static blobs.\n"},
		{Path: "resources/certs/README.md", Language: "markdown", Content: "# Certificates\n\nUse this folder for local non-secret certificates that support TLS and mTLS flows.\n"},
	}
}

func renderSuiteStar(name string) string {
	return `load("@babelsuite/runtime", "service", "task", "test", "traffic", "suite")

db = service.run(
    image="postgres:16",
)

api = service.run(
    image="ghcr.io/acme/sample-api:latest",
    after=[db],
    env={
        "DATABASE_URL": "postgres://postgres:postgres@db:5432/app",
    },
)

catalog = service.mock(
    contract="api/openapi.yaml",
    source="mock/catalog",
    after=[api],
)

bootstrap = task.run(
    file="bootstrap.sh",
    image="bash:5.2",
    after=[api],
)

smoke_traffic = traffic.smoke(
    plan="smoke.star",
    target="http://api:8080",
    after=[bootstrap],
)

api_smoke = test.run(
    image="curlimages/curl:8.7.1",
    file="http/smoke.hurl",
    env={
        "BASE_URL": "http://api:8080",
    },
    after=[smoke_traffic, catalog],
)
`
}

func renderMetadataYAML(name string, title string) string {
	return `name: ` + name + `
title: ` + title + `
labels:
  owner: platform
  tier: starter
tags:
  - starter
  - local
`
}

func renderLocalProfile(name string, title string) string {
	return `name: Local
description: Default local profile for ` + title + `.
default: true
runtime:
  suite: ` + name + `
  repository: localhost:5000/local/` + name + `
  profileFile: local.yaml
env:
  BASE_URL: http://api:8080
  SUITE_MODE: local
observability:
  logs: structured
  traces: enabled
  metrics: enabled
services:
  api:
    env:
      FEATURE_FLAG_SAMPLE: enabled
`
}

func renderOpenAPI(title string) string {
	return `openapi: 3.1.0
info:
  title: ` + title + ` API
  version: 0.1.0
paths:
  /health:
    get:
      operationId: getHealth
      summary: Health probe
      responses:
        "200":
          description: healthy
          content:
            application/json:
              schema:
                type: object
                properties:
                  status:
                    type: string
                  service:
                    type: string
`
}

func renderMockCue() string {
	return `package catalog

examples: {
  GetItem: {
    request: {
      method: "GET"
      path:   "/catalog/items/sku-123"
    }
    response: {
      status:    "200"
      mediaType: "application/json"
      body: {
        sku:      "sku-123"
        name:     "Starter item"
        price:    42
        currency: "USD"
        profile:  string @resolve(path="request.headers.x-suite-profile", fallback="local.yaml")
      }
    }
  }
}
`
}

func renderMockMetadata(name string) string {
	return `apiVersion: mocks.babelsuite.io/v1alpha1
kind: MockOperation
metadata:
  metadataPath: mock/catalog/get-item.metadata.yaml
  operationId: get-item
  sourceArtifact: mock/catalog/get-item.cue
  suite: ` + name + `
spec:
  adapter: rest
  dispatcher: path
  resolverUrl: /internal/mock-data/` + name + `/catalog/get-item
  runtimeUrl: /mocks/rest/` + name + `/catalog/items/sku-123
`
}

func renderBootstrapScript() string {
	return "#!/usr/bin/env bash\nset -euo pipefail\n\necho \"Bootstrapping starter suite assets\"\n"
}

func renderLoadPlan() string {
	return `load("@babelsuite/runtime", "traffic")

probe = traffic.user(
    name="probe",
    weight=1,
    wait=traffic.constant(1),
    tasks=[
        traffic.task(
            name="health",
            request=traffic.get("/health", name="health"),
            checks=[
                traffic.threshold("status", "==", 200),
                traffic.threshold("latency.p95_ms", "<", 500, sampler="health"),
            ],
        ),
    ],
)

traffic.plan(
    users=[probe],
    shape=traffic.stages([
        traffic.stage(duration="30s", users=5, spawn_rate=2),
        traffic.stage(duration="1m", users=10, spawn_rate=5),
        traffic.stage(duration="90s", users=0, spawn_rate=5, stop=True),
    ]),
    thresholds=[
        traffic.threshold("http.error_rate", "<", 0.01),
        traffic.threshold("http.p95_ms", "<", 500, sampler="health"),
    ],
)
`
}

func renderDocsReadme(name string, title string) string {
	return `# ` + title + `

This starter suite was generated by ` + "`babelctl create`" + `.

## What is included

- ` + "`metadata.yaml`" + `: optional suite metadata, labels, and tags
- ` + "`suite.star`" + `: the runtime topology
- ` + "`profiles/local.yaml`" + `: a launch profile with runtime env
- ` + "`api/`" + `: starter contract assets
- ` + "`mock/`" + `: native mock behavior
- ` + "`services/`" + `: background infrastructure assets
- ` + "`tasks/`" + `: short-lived setup jobs
- ` + "`tests/`" + `: verification assets
- ` + "`traffic/`" + `: a native traffic plan
- ` + "`resources/`" + `: passive certs and data assets

## Suggested next steps

1. Replace the sample container image in ` + "`suite.star`" + `.
2. Adjust the profile env values in ` + "`profiles/local.yaml`" + `.
3. Replace the starter mock and test assets with your real suite behavior.
4. Run the suite with ` + "`babelctl run " + name + "`" + ` once the package is available in your environment.
`
}

func normalizeTemplateName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	lastDash := false

	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			builder.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || r == ' ' || r == '/':
			if builder.Len() > 0 && !lastDash {
				builder.WriteByte('-')
				lastDash = true
			}
		}
	}

	result := strings.Trim(builder.String(), "-")
	return result
}

func titleizeTemplateName(value string) string {
	value = strings.ReplaceAll(value, "-", " ")
	value = strings.ReplaceAll(value, "_", " ")
	parts := strings.Fields(value)
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
	}
	if len(parts) == 0 {
		return "Starter Suite"
	}
	return strings.Join(parts, " ")
}

func printHelp(rt *support.Runtime) {
	_, _ = fmt.Fprintln(rt.Stdout, `Usage:
  babelctl create <name> [destination] [--force] [--title <title>]
  babelctl create template <name> [destination] [--force] [--title <title>]

Create a starter suite template on disk using BabelSuite's native suite layout.`)
}
