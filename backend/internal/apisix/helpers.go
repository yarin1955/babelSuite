package apisix

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

func publicPath(operation OperationConfig) string {
	if strings.HasPrefix(strings.TrimSpace(operation.Name), "/") {
		return strings.TrimSpace(operation.Name)
	}
	if raw := strings.TrimSpace(operation.MockURL); raw != "" {
		if parsed, err := url.Parse(raw); err == nil && strings.TrimSpace(parsed.Path) != "" {
			return parsed.Path
		}
	}
	if strings.TrimSpace(operation.Name) != "" {
		return "/" + strings.Trim(strings.TrimSpace(operation.Name), "/")
	}
	return "/" + strings.TrimSpace(operation.ID)
}

func matchURI(operation OperationConfig) string {
	path := publicPath(operation)
	if !hasPathParams(path) {
		return path
	}

	replaced := pathParameterPattern.ReplaceAllString(path, "*")
	if strings.TrimSpace(replaced) == "" {
		return "/*"
	}
	return replaced
}

func proxyRewrite(suiteID, surfaceID string, operation OperationConfig) map[string]any {
	headers := map[string]string{
		"X-Babelsuite-Dispatcher": "apisix",
		"X-Babelsuite-Operation":  operation.ID,
	}

	target := runtimeTargetPath(suiteID, surfaceID, operation)
	if !hasPathParams(publicPath(operation)) {
		return map[string]any{
			"uri": target,
			"headers": map[string]any{
				"set": headers,
			},
		}
	}

	pattern, replacement := rewritePattern(publicPath(operation), target)
	return map[string]any{
		"regex_uri": []string{pattern, replacement},
		"headers": map[string]any{
			"set": headers,
		},
	}
}

func resolverPlugin(transport string, operation OperationConfig) map[string]any {
	value, _ := json.Marshal(map[string]string{
		"resolver_url":      resolverPath(operation.MockMetadata.ResolverURL),
		"public_path":       publicPath(operation),
		"protocol":          strings.ToUpper(strings.TrimSpace(transport)),
		"operation_id":      operation.ID,
		"compatibility_url": runtimePath(operation.MockMetadata.RuntimeURL),
	})
	return map[string]any{
		"allow_degradation": true,
		"conf": []map[string]any{
			{
				"name":  "babelsuite-resolver",
				"value": string(value),
			},
		},
	}
}

func httpMethod(operation OperationConfig) string {
	method := strings.ToUpper(strings.TrimSpace(operation.Method))
	if method == "" || method == "RPC" || method == "EVENT" {
		return "POST"
	}
	return method
}

func routeHosts(surface SurfaceConfig) []string {
	host := strings.TrimSpace(hostOnly(surface.MockHost))
	if host == "" {
		return nil
	}
	return []string{host}
}

func runtimePath(runtimeURL string) string {
	raw := strings.TrimSpace(runtimeURL)
	if raw == "" {
		return "/"
	}
	if parsed, err := url.Parse(raw); err == nil && strings.TrimSpace(parsed.Path) != "" {
		return parsed.Path
	}
	if index := strings.Index(raw, "?"); index >= 0 {
		return raw[:index]
	}
	return raw
}

func resolverPath(resolverURL string) string {
	return runtimePath(resolverURL)
}

func hostOnly(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if parsed, err := url.Parse(trimmed); err == nil {
		if host := strings.TrimSpace(parsed.Host); host != "" {
			return host
		}
	}
	return strings.Trim(trimmed, "/")
}

func hasPathParams(path string) bool {
	return pathParameterPattern.MatchString(path)
}

func rewritePattern(publicPath, targetPath string) (string, string) {
	matches := pathParameterPattern.FindAllString(publicPath, -1)
	pattern := "^" + pathParameterPattern.ReplaceAllString(publicPath, "([^/]+)") + "$"
	replacement := targetPath
	for index := range matches {
		replacement = strings.Replace(replacement, matches[index], fmt.Sprintf("$%d", index+1), 1)
	}

	return pattern, replacement
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func runtimeTargetPath(suiteID, surfaceID string, operation OperationConfig) string {
	path := strings.TrimSpace(operation.Name)
	if path == "" {
		path = publicPath(operation)
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + strings.Trim(path, "/")
	}
	return "/mocks/rest/" + strings.Trim(suiteID, "/") + "/" + strings.Trim(surfaceID, "/") + path
}

func contractSourcePath(contractPath string) string {
	base, _, _ := strings.Cut(strings.TrimSpace(contractPath), "#")
	return strings.TrimSpace(base)
}

func ensureTrailingNewline(value string) string {
	if value == "" || strings.HasSuffix(value, "\n") {
		return value
	}
	return value + "\n"
}
