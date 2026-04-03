package mocking

import (
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/babelsuite/babelsuite/internal/suites"
)

func PreviewExchange(suite suites.Definition, surface suites.APISurface, operation suites.APIOperation, exchange suites.ExchangeExample) suites.ExchangeExample {
	preview := exchange
	preview.When = append([]suites.MatchCondition{}, exchange.When...)
	preview.RequestHeaders = append([]suites.Header{}, exchange.RequestHeaders...)
	preview.ResponseHeaders = append([]suites.Header{}, exchange.ResponseHeaders...)

	snapshot := previewRequestSnapshot(operation, exchange)
	state := previewState(operation.MockMetadata.State)
	for _, example := range loadSchemaExamples(suite, operation) {
		if example.Name != exchange.Name {
			continue
		}
		result := buildSchemaResult(suite, surface, operation, snapshot, state, operation.MockMetadata.Adapter, example)
		preview.ResponseMediaType = firstNonEmpty(result.MediaType, preview.ResponseMediaType)
		preview.ResponseHeaders = previewResponseHeaders(result.Headers)
		preview.ResponseBody = string(result.Body)
		return preview
	}

	contextMap := buildTemplateContext(suite, surface, operation, snapshot, state, nil, exchange.Name)
	preview.ResponseHeaders = renderPreviewHeaders(exchange.ResponseHeaders, contextMap)
	preview.ResponseBody = renderTemplate(exchange.ResponseBody, contextMap)
	preview.ResponseMediaType = firstNonEmpty(preview.ResponseMediaType, inferredMediaType(preview.ResponseBody))
	return preview
}

func previewRequestSnapshot(operation suites.APIOperation, exchange suites.ExchangeExample) requestSnapshot {
	snapshot := requestSnapshot{
		Method:  strings.TrimSpace(operation.Method),
		Query:   map[string]string{},
		Headers: map[string]string{},
		Path:    map[string]string{},
		BodyRaw: exchange.RequestBody,
	}

	if parsedURL, err := url.Parse(strings.TrimSpace(operation.MockURL)); err == nil {
		for key, value := range flattenValues(parsedURL.Query()) {
			snapshot.Query[key] = value
		}
		if pathParams, ok := matchOperationPath(operation.Name, parsedURL.Path); ok {
			for key, value := range pathParams {
				snapshot.Path[key] = value
			}
		}
	}

	for _, header := range exchange.RequestHeaders {
		if strings.TrimSpace(header.Name) == "" {
			continue
		}
		snapshot.Headers[strings.ToLower(header.Name)] = header.Value
	}

	if strings.TrimSpace(exchange.RequestBody) != "" {
		var bodyJSON any
		if err := json.Unmarshal([]byte(exchange.RequestBody), &bodyJSON); err == nil {
			snapshot.BodyJSON = bodyJSON
			snapshot.BodyObject, _ = bodyJSON.(map[string]any)
		} else {
			snapshot.BodyJSON = strings.TrimSpace(exchange.RequestBody)
		}
	}

	for _, condition := range exchange.When {
		switch strings.ToLower(strings.TrimSpace(condition.From)) {
		case "path":
			snapshot.Path[condition.Param] = condition.Value
		case "header":
			snapshot.Headers[strings.ToLower(condition.Param)] = condition.Value
		case "body":
			snapshot.BodyObject = setPreviewBodyValue(snapshot.BodyObject, condition.Param, condition.Value)
			snapshot.BodyJSON = snapshot.BodyObject
		default:
			snapshot.Query[condition.Param] = condition.Value
		}
	}

	return snapshot
}

func previewState(config *suites.MockState) map[string]string {
	if config == nil {
		return nil
	}
	return cloneStringMap(config.Defaults)
}

func setPreviewBodyValue(body map[string]any, path, value string) map[string]any {
	if body == nil {
		body = map[string]any{}
	}

	segments := strings.Split(strings.TrimSpace(path), ".")
	if len(segments) == 0 {
		return body
	}

	current := body
	for index, segment := range segments {
		if segment == "" {
			continue
		}
		if index == len(segments)-1 {
			current[segment] = value
			return body
		}

		next, _ := current[segment].(map[string]any)
		if next == nil {
			next = map[string]any{}
			current[segment] = next
		}
		current = next
	}

	return body
}

func renderPreviewHeaders(headers []suites.Header, contextMap map[string]any) []suites.Header {
	rendered := make([]suites.Header, len(headers))
	for index, header := range headers {
		rendered[index] = suites.Header{
			Name:  header.Name,
			Value: renderTemplate(header.Value, contextMap),
		}
	}
	return rendered
}

func previewResponseHeaders(headers http.Header) []suites.Header {
	if len(headers) == 0 {
		return nil
	}

	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	output := make([]suites.Header, 0, len(keys))
	for _, key := range keys {
		values := headers.Values(key)
		if len(values) == 0 {
			continue
		}
		output = append(output, suites.Header{
			Name:  key,
			Value: strings.Join(values, ", "),
		})
	}
	return output
}
