package mocking

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/babelsuite/babelsuite/internal/suites"
)

var (
	ErrSurfaceNotFound   = errors.New("mock surface not found")
	ErrOperationNotFound = errors.New("mock operation not found")
)

type suiteReader interface {
	Get(id string) (*suites.Definition, error)
}

type Service struct {
	suites suiteReader

	mu    sync.RWMutex
	state map[string]map[string]string
}

type Result struct {
	Status         int
	MediaType      string
	Headers        http.Header
	Body           []byte
	Adapter        string
	RuntimeURL     string
	MatchedExample string
}

type requestSnapshot struct {
	Method     string
	Query      map[string]string
	Headers    map[string]string
	Path       map[string]string
	BodyRaw    string
	BodyJSON   any
	BodyObject map[string]any
}

func NewService(suiteService suiteReader) *Service {
	return &Service{
		suites: suiteService,
		state:  make(map[string]map[string]string),
	}
}

func (s *Service) InvokeREST(ctx context.Context, suiteID, surfaceID, requestPath string, request *http.Request) (*Result, error) {
	suite, surface, operation, pathParams, err := s.lookupRESTOperation(suiteID, surfaceID, request.Method, requestPath)
	if err != nil {
		return nil, err
	}
	return s.resolve(ctx, suite, surface, operation, pathParams, request, "rest")
}

func (s *Service) InvokeAdapter(ctx context.Context, suiteID, surfaceID, operationID, adapter string, request *http.Request) (*Result, error) {
	suite, err := s.suites.Get(suiteID)
	if err != nil {
		return nil, err
	}

	surface, operation, err := findOperationByID(*suite, surfaceID, operationID)
	if err != nil {
		return nil, err
	}

	return s.resolve(ctx, suite, surface, operation, map[string]string{}, request, strings.ToLower(strings.TrimSpace(adapter)))
}

func (s *Service) lookupRESTOperation(suiteID, surfaceID, method, requestPath string) (*suites.Definition, suites.APISurface, suites.APIOperation, map[string]string, error) {
	suite, err := s.suites.Get(suiteID)
	if err != nil {
		return nil, suites.APISurface{}, suites.APIOperation{}, nil, err
	}

	for _, surface := range suite.APISurfaces {
		if surface.ID != strings.TrimSpace(surfaceID) {
			continue
		}
		for _, operation := range surface.Operations {
			if !strings.EqualFold(operation.MockMetadata.Adapter, "rest") && !strings.EqualFold(surface.Protocol, "REST") {
				continue
			}
			if !strings.EqualFold(strings.TrimSpace(operation.Method), strings.TrimSpace(method)) {
				continue
			}
			params, ok := matchOperationPath(operation.Name, requestPath)
			if !ok {
				continue
			}
			return suite, surface, operation, params, nil
		}
		return nil, suites.APISurface{}, suites.APIOperation{}, nil, ErrOperationNotFound
	}

	return nil, suites.APISurface{}, suites.APIOperation{}, nil, ErrSurfaceNotFound
}

func findOperationByID(suite suites.Definition, surfaceID, operationID string) (suites.APISurface, suites.APIOperation, error) {
	for _, surface := range suite.APISurfaces {
		if surface.ID != strings.TrimSpace(surfaceID) {
			continue
		}
		for _, operation := range surface.Operations {
			if operation.ID == strings.TrimSpace(operationID) {
				return surface, operation, nil
			}
		}
		return suites.APISurface{}, suites.APIOperation{}, ErrOperationNotFound
	}
	return suites.APISurface{}, suites.APIOperation{}, ErrSurfaceNotFound
}

func (s *Service) resolve(ctx context.Context, suite *suites.Definition, surface suites.APISurface, operation suites.APIOperation, pathParams map[string]string, request *http.Request, adapter string) (*Result, error) {
	requestBody, bodyJSON, bodyObject, err := readRequestBody(request)
	if err != nil {
		return errorResult(http.StatusBadRequest, "application/json", fmt.Sprintf(`{"error":"%s"}`, escapeJSONString("Could not read request body."))), nil
	}

	snapshot := requestSnapshot{
		Method:     request.Method,
		Query:      flattenValues(request.URL.Query()),
		Headers:    flattenHeader(request.Header),
		Path:       pathParams,
		BodyRaw:    requestBody,
		BodyJSON:   bodyJSON,
		BodyObject: bodyObject,
	}

	if result := validateConstraints(operation.MockMetadata.ParameterConstraints, snapshot); result != nil {
		return result, nil
	}

	stateKey := ""
	if operation.MockMetadata.State != nil {
		stateKey = renderTemplate(operation.MockMetadata.State.LookupKeyTemplate, buildTemplateContext(*suite, surface, operation, snapshot, nil, nil, ""))
	}
	state := s.loadState(stateKey, operation.MockMetadata.State)
	contextMap := buildTemplateContext(*suite, surface, operation, snapshot, state, nil, "")

	example, ok := matchExample(operation, snapshot)
	if !ok {
		return s.resolveFallback(ctx, *suite, surface, operation, snapshot, state, request)
	}

	if operation.MockMetadata.DelayMillis > 0 {
		time.Sleep(time.Duration(operation.MockMetadata.DelayMillis) * time.Millisecond)
	}

	responseHeaders := make(http.Header)
	for _, header := range example.ResponseHeaders {
		responseHeaders.Set(header.Name, renderTemplate(header.Value, contextMap))
	}
	applyRecopiedConstraintHeaders(responseHeaders, operation.MockMetadata.ParameterConstraints, snapshot)

	renderedBody := renderTemplate(example.ResponseBody, contextMap)
	responseStatus := parseStatusCode(example.ResponseStatus, http.StatusOK)
	result := &Result{
		Status:         responseStatus,
		MediaType:      firstNonEmpty(example.ResponseMediaType, inferredMediaType(renderedBody)),
		Headers:        responseHeaders,
		Body:           []byte(renderedBody),
		Adapter:        adapter,
		RuntimeURL:     operation.MockMetadata.RuntimeURL,
		MatchedExample: example.Name,
	}

	s.applyStateTransition(operation.MockMetadata.State, stateKey, example.Name, *suite, surface, operation, snapshot, state, result)
	return result, nil
}

func (s *Service) resolveFallback(ctx context.Context, suite suites.Definition, surface suites.APISurface, operation suites.APIOperation, snapshot requestSnapshot, state map[string]string, request *http.Request) (*Result, error) {
	fallback := operation.MockMetadata.Fallback
	if fallback == nil {
		return errorResult(http.StatusNotFound, "application/json", fmt.Sprintf(`{"error":"%s"}`, escapeJSONString("No matching mock exchange was found."))), nil
	}

	switch strings.ToLower(strings.TrimSpace(fallback.Mode)) {
	case "example":
		for _, example := range operation.Exchanges {
			if example.Name != fallback.ExampleName {
				continue
			}
			contextMap := buildTemplateContext(suite, surface, operation, snapshot, state, nil, example.Name)
			headers := make(http.Header)
			for _, header := range example.ResponseHeaders {
				headers.Set(header.Name, renderTemplate(header.Value, contextMap))
			}
			applyRecopiedConstraintHeaders(headers, operation.MockMetadata.ParameterConstraints, snapshot)
			return &Result{
				Status:         parseStatusCode(example.ResponseStatus, http.StatusOK),
				MediaType:      firstNonEmpty(example.ResponseMediaType, inferredMediaType(example.ResponseBody)),
				Headers:        headers,
				Body:           []byte(renderTemplate(example.ResponseBody, contextMap)),
				Adapter:        operation.MockMetadata.Adapter,
				RuntimeURL:     operation.MockMetadata.RuntimeURL,
				MatchedExample: example.Name,
			}, nil
		}
	case "proxy":
		return proxyFallback(ctx, request, fallback, operation.MockMetadata.RuntimeURL)
	default:
		contextMap := buildTemplateContext(suite, surface, operation, snapshot, state, nil, "")
		headers := make(http.Header)
		for _, header := range fallback.Headers {
			headers.Set(header.Name, renderTemplate(header.Value, contextMap))
		}
		applyRecopiedConstraintHeaders(headers, operation.MockMetadata.ParameterConstraints, snapshot)
		return &Result{
			Status:     parseStatusCode(fallback.Status, http.StatusBadRequest),
			MediaType:  firstNonEmpty(fallback.MediaType, inferredMediaType(fallback.Body)),
			Headers:    headers,
			Body:       []byte(renderTemplate(fallback.Body, contextMap)),
			Adapter:    operation.MockMetadata.Adapter,
			RuntimeURL: operation.MockMetadata.RuntimeURL,
		}, nil
	}

	return errorResult(http.StatusNotFound, "application/json", fmt.Sprintf(`{"error":"%s"}`, escapeJSONString("Fallback example was not found."))), nil
}

func proxyFallback(ctx context.Context, original *http.Request, fallback *suites.MockFallback, runtimeURL string) (*Result, error) {
	target := strings.TrimSpace(fallback.ProxyURL)
	if target == "" {
		return errorResult(http.StatusBadGateway, "application/json", `{"error":"Fallback proxy URL is not configured."}`), nil
	}

	body, _, _, err := readRequestBody(original)
	if err != nil {
		return errorResult(http.StatusBadRequest, "application/json", `{"error":"Could not replay request body for fallback proxy."}`), nil
	}

	req, err := http.NewRequestWithContext(ctx, original.Method, target, bytes.NewReader([]byte(body)))
	if err != nil {
		return nil, err
	}
	req.Header = original.Header.Clone()
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return errorResult(http.StatusBadGateway, "application/json", fmt.Sprintf(`{"error":"%s"}`, escapeJSONString(err.Error()))), nil
	}
	defer response.Body.Close()

	responseBody, _ := io.ReadAll(response.Body)
	headers := response.Header.Clone()
	headers.Set("X-Babelsuite-Fallback", "proxy")
	headers.Set("X-Babelsuite-Runtime-Url", runtimeURL)
	return &Result{
		Status:     response.StatusCode,
		MediaType:  response.Header.Get("Content-Type"),
		Headers:    headers,
		Body:       responseBody,
		Adapter:    "proxy",
		RuntimeURL: runtimeURL,
	}, nil
}

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
		return trimmed, nil, nil, nil
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

func (s *Service) loadState(key string, config *suites.MockState) map[string]string {
	state := cloneStringMap(nil)
	if config != nil {
		state = cloneStringMap(config.Defaults)
	}
	if strings.TrimSpace(key) == "" {
		return state
	}

	s.mu.RLock()
	current, ok := s.state[key]
	s.mu.RUnlock()
	if !ok {
		return state
	}

	merged := cloneStringMap(state)
	if merged == nil {
		merged = make(map[string]string, len(current))
	}
	for field, value := range current {
		merged[field] = value
	}
	return merged
}

func (s *Service) applyStateTransition(config *suites.MockState, lookupKey, exampleName string, suite suites.Definition, surface suites.APISurface, operation suites.APIOperation, snapshot requestSnapshot, state map[string]string, result *Result) {
	if config == nil {
		return
	}

	responseBodyJSON := parseJSONMap(string(result.Body))
	contextMap := buildTemplateContext(suite, surface, operation, snapshot, state, responseBodyJSON, exampleName)

	for _, transition := range config.Transitions {
		if strings.TrimSpace(transition.OnExample) != "" && transition.OnExample != exampleName {
			continue
		}

		key := renderTemplate(firstNonEmpty(transition.MutationKeyTemplate, config.MutationKeyTemplate, lookupKey), contextMap)
		if strings.TrimSpace(key) == "" {
			continue
		}

		nextState := cloneStringMap(state)
		if nextState == nil {
			nextState = make(map[string]string)
		}
		for field, value := range transition.Set {
			nextState[field] = renderTemplate(value, contextMap)
		}
		for _, field := range transition.Delete {
			delete(nextState, field)
		}
		for field, delta := range transition.Increment {
			current, _ := strconv.Atoi(nextState[field])
			nextState[field] = strconv.Itoa(current + delta)
		}
		s.storeState(key, nextState)
		result.Headers.Set("X-Babelsuite-State-Key", key)
		return
	}
}

func (s *Service) storeState(key string, value map[string]string) {
	if strings.TrimSpace(key) == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.state[key] = cloneStringMap(value)
}

func buildTemplateContext(suite suites.Definition, surface suites.APISurface, operation suites.APIOperation, snapshot requestSnapshot, state map[string]string, responseBody map[string]any, exampleName string) map[string]any {
	return map[string]any{
		"suite": map[string]any{
			"id":           suite.ID,
			"title":        suite.Title,
			"repository":   suite.Repository,
			"surfaceId":    surface.ID,
			"surfaceTitle": surface.Title,
		},
		"operation": map[string]any{
			"id":         operation.ID,
			"name":       operation.Name,
			"runtimeUrl": operation.MockMetadata.RuntimeURL,
		},
		"request": map[string]any{
			"method":  snapshot.Method,
			"query":   snapshot.Query,
			"path":    snapshot.Path,
			"headers": snapshot.Headers,
			"body":    firstNonNil(snapshot.BodyJSON, snapshot.BodyObject),
			"rawBody": snapshot.BodyRaw,
		},
		"response": map[string]any{
			"body": responseBody,
		},
		"state": firstNonNil(state, map[string]string{}),
		"match": map[string]any{"example": exampleName},
	}
}

func renderTemplate(input string, contextMap map[string]any) string {
	if strings.TrimSpace(input) == "" {
		return input
	}

	pattern := regexp.MustCompile(`\{\{\s*([a-zA-Z0-9._-]+)\s*\}\}`)
	return pattern.ReplaceAllStringFunc(input, func(token string) string {
		match := pattern.FindStringSubmatch(token)
		if len(match) != 2 {
			return token
		}
		value, ok := lookupContextValue(contextMap, match[1])
		if !ok {
			return ""
		}
		return fmt.Sprint(value)
	})
}

func lookupContextValue(value any, path string) (any, bool) {
	current := value
	for _, segment := range strings.Split(strings.TrimSpace(path), ".") {
		switch typed := current.(type) {
		case map[string]any:
			next, ok := typed[segment]
			if !ok {
				return nil, false
			}
			current = next
		case map[string]string:
			next, ok := typed[segment]
			if !ok {
				return nil, false
			}
			current = next
		default:
			return nil, false
		}
	}
	return current, true
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

func bodyLookup(body any, key string) string {
	if body == nil || strings.TrimSpace(key) == "" {
		return ""
	}

	current := body
	for _, segment := range strings.Split(key, ".") {
		switch typed := current.(type) {
		case map[string]any:
			current = typed[segment]
		default:
			return ""
		}
	}
	if current == nil {
		return ""
	}
	return fmt.Sprint(current)
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
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
