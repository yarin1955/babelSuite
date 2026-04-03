package mocking

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/babelsuite/babelsuite/internal/suites"
)

func readRequestBody(request *http.Request) (string, any, map[string]any, error) {
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return "", nil, nil, err
	}
	request.Body = io.NopCloser(bytes.NewReader(body))

	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "", nil, nil, nil
	}

	var parsed any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return trimmed, trimmed, nil, nil
	}
	object, _ := parsed.(map[string]any)
	return trimmed, parsed, object, nil
}

func validateConstraints(constraints []suites.ParameterConstraint, snapshot requestSnapshot) *Result {
	for _, constraint := range constraints {
		value := requestValueAt(constraint.Source, constraint.Name, snapshot)
		if constraint.Required && strings.TrimSpace(value) == "" {
			return errorResult(http.StatusBadRequest, "application/json", fmt.Sprintf(`{"error":"%s"}`, escapeJSONString(fmt.Sprintf("Missing required %s parameter %q.", constraint.Source, constraint.Name))))
		}
		if strings.TrimSpace(value) == "" || strings.TrimSpace(constraint.Pattern) == "" {
			continue
		}
		expression, err := regexp.Compile(constraint.Pattern)
		if err != nil {
			continue
		}
		if !expression.MatchString(value) {
			return errorResult(http.StatusBadRequest, "application/json", fmt.Sprintf(`{"error":"%s"}`, escapeJSONString(fmt.Sprintf("%s parameter %q failed validation.", strings.Title(constraint.Source), constraint.Name))))
		}
	}
	return nil
}

func matchExample(operation suites.APIOperation, snapshot requestSnapshot) (suites.ExchangeExample, bool) {
	if len(operation.Exchanges) == 1 && len(operation.Exchanges[0].When) == 0 {
		return operation.Exchanges[0], true
	}
	for _, example := range operation.Exchanges {
		if matchWhen(example.When, snapshot) {
			return example, true
		}
	}
	return suites.ExchangeExample{}, false
}

func matchWhen(conditions []suites.MatchCondition, snapshot requestSnapshot) bool {
	for _, cond := range conditions {
		if valueAt(cond.From, cond.Param, snapshot) != cond.Value {
			return false
		}
	}
	return true
}

func valueAt(from, param string, snapshot requestSnapshot) string {
	switch strings.ToLower(strings.TrimSpace(from)) {
	case "path":
		return snapshot.Path[param]
	case "header":
		return snapshot.Headers[strings.ToLower(param)]
	case "body":
		return bodyLookup(snapshot.BodyJSON, param)
	default:
		return snapshot.Query[param]
	}
}

func requestValueAt(location, key string, snapshot requestSnapshot) string {
	switch strings.ToLower(strings.TrimSpace(location)) {
	case "path":
		return snapshot.Path[key]
	case "header":
		return snapshot.Headers[strings.ToLower(key)]
	default:
		return snapshot.Query[key]
	}
}

func applyRecopiedConstraintHeaders(headers http.Header, constraints []suites.ParameterConstraint, snapshot requestSnapshot) {
	for _, constraint := range constraints {
		if !constraint.Forward {
			continue
		}
		value := requestValueAt(constraint.Source, constraint.Name, snapshot)
		if strings.TrimSpace(value) == "" {
			continue
		}
		headers.Set(constraint.Name, value)
	}
}

func parseJSONMap(body string) map[string]any {
	if strings.TrimSpace(body) == "" {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return nil
	}
	return payload
}

func cloneResolverRequest(ctx context.Context, request *http.Request, operation suites.APIOperation, originalPath string) *http.Request {
	clone := request.Clone(ctx)
	clone.Method = resolverOriginalMethod(request, operation)
	if clone.URL == nil {
		clone.URL = &url.URL{}
	} else {
		urlCopy := *clone.URL
		clone.URL = &urlCopy
	}
	clone.URL.Path = originalPath
	return clone
}

func resolverOriginalMethod(request *http.Request, operation suites.APIOperation) string {
	if method := strings.ToUpper(strings.TrimSpace(request.Header.Get("X-Babelsuite-Original-Method"))); method != "" {
		return method
	}
	if method := strings.ToUpper(strings.TrimSpace(request.Method)); method != "" && method != http.MethodPost {
		return method
	}
	method := strings.ToUpper(strings.TrimSpace(operation.Method))
	if method == "" || method == "RPC" || method == "EVENT" {
		return http.MethodPost
	}
	return method
}

func resolverOriginalPath(request *http.Request, operation suites.APIOperation) string {
	for _, candidate := range []string{
		request.Header.Get("X-Babelsuite-Original-Path"),
		request.URL.Query().Get("_path"),
		operation.MockURL,
		operation.Name,
	} {
		if path := normalizedResolverPath(candidate); path != "" {
			return path
		}
	}
	if strings.TrimSpace(operation.ID) == "" {
		return "/"
	}
	return "/" + strings.Trim(strings.TrimSpace(operation.ID), "/")
}

func normalizedResolverPath(candidate string) string {
	raw := strings.TrimSpace(candidate)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "/") {
		return raw
	}
	if parsed, err := url.Parse(raw); err == nil && strings.TrimSpace(parsed.Path) != "" {
		return parsed.Path
	}
	return "/" + strings.Trim(raw, "/")
}

func flattenValues(input map[string][]string) map[string]string {
	output := make(map[string]string, len(input))
	for key, values := range input {
		if len(values) == 0 {
			continue
		}
		output[key] = values[0]
	}
	return output
}

func flattenHeader(input http.Header) map[string]string {
	output := make(map[string]string, len(input))
	for key, values := range input {
		if len(values) == 0 {
			continue
		}
		output[strings.ToLower(key)] = values[0]
	}
	return output
}

func matchOperationPath(pattern, path string) (map[string]string, bool) {
	left := splitPath(pattern)
	right := splitPath(path)
	if len(left) != len(right) {
		return nil, false
	}

	params := make(map[string]string)
	for index := range left {
		if strings.HasPrefix(left[index], "{") && strings.HasSuffix(left[index], "}") {
			params[strings.TrimSuffix(strings.TrimPrefix(left[index], "{"), "}")] = right[index]
			continue
		}
		if left[index] != right[index] {
			return nil, false
		}
	}
	return params, true
}

func splitPath(path string) []string {
	trimmed := strings.Trim(strings.TrimSpace(path), "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func parseStatusCode(value string, fallback int) int {
	status, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || status <= 0 {
		return fallback
	}
	return status
}

func inferredMediaType(body string) string {
	trimmed := strings.TrimSpace(body)
	switch {
	case strings.HasPrefix(trimmed, "{"), strings.HasPrefix(trimmed, "["):
		return "application/json"
	case strings.HasPrefix(trimmed, "<"):
		return "application/xml"
	default:
		return "text/plain; charset=utf-8"
	}
}

func errorResult(status int, mediaType, body string) *Result {
	headers := make(http.Header)
	return &Result{
		Status:    status,
		MediaType: mediaType,
		Headers:   headers,
		Body:      []byte(body),
	}
}

func withOperationMetadata(result *Result, operation suites.APIOperation, adapter string) *Result {
	if result == nil {
		return nil
	}
	if result.Headers == nil {
		result.Headers = make(http.Header)
	}
	if strings.TrimSpace(result.Adapter) == "" {
		result.Adapter = strings.TrimSpace(adapter)
	}
	if strings.TrimSpace(result.Dispatcher) == "" {
		result.Dispatcher = firstNonEmpty(operation.Dispatcher, operation.MockMetadata.Dispatcher)
	}
	if strings.TrimSpace(result.ResolverURL) == "" {
		result.ResolverURL = operation.MockMetadata.ResolverURL
	}
	if strings.TrimSpace(result.RuntimeURL) == "" {
		result.RuntimeURL = operation.MockMetadata.RuntimeURL
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func escapeJSONString(input string) string {
	body, _ := json.Marshal(input)
	return strings.Trim(string(body), `"`)
}

func sortedKeys(input map[string]string) []string {
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
