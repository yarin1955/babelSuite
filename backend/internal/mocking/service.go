package mocking

import (
	"bytes"
	"context"
	cryptoRand "crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/babelsuite/babelsuite/internal/suites"
	"github.com/google/uuid"
)

var (
	ErrSurfaceNotFound   = errors.New("mock surface not found")
	ErrOperationNotFound = errors.New("mock operation not found")

	templateTokenPattern = regexp.MustCompile(`(?s)\{\{\s*(.*?)\s*\}\}`)
	functionCallPattern  = regexp.MustCompile(`^([a-zA-Z_$][a-zA-Z0-9_$-]*)\((.*)\)$`)
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
	Dispatcher     string
	ResolverURL    string
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

type schemaExampleDocument struct {
	Examples map[string]schemaExampleEntry `json:"examples"`
}

type schemaExampleEntry struct {
	Dispatch       []suites.MatchCondition `json:"dispatch"`
	RequestSchema  schemaRequestSpec       `json:"requestSchema"`
	ResponseSchema schemaResponseSpec      `json:"responseSchema"`
}

type schemaRequestSpec struct {
	Headers any `json:"headers"`
	Body    any `json:"body"`
}

type schemaResponseSpec struct {
	Status    string `json:"status"`
	MediaType string `json:"mediaType"`
	Headers   any    `json:"headers"`
	Body      any    `json:"body"`
}

type schemaBackedExample struct {
	Name     string
	Dispatch []suites.MatchCondition
	Request  schemaRequestSpec
	Response schemaResponseSpec
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

func (s *Service) ResolveOperation(ctx context.Context, suiteID, surfaceID, operationID string, request *http.Request) (*Result, error) {
	suite, err := s.suites.Get(suiteID)
	if err != nil {
		return nil, err
	}

	surface, operation, err := findOperationByID(*suite, surfaceID, operationID)
	if err != nil {
		return nil, err
	}

	adapter := strings.ToLower(strings.TrimSpace(operation.MockMetadata.Adapter))
	originalPath := resolverOriginalPath(request, operation)
	pathParams := map[string]string{}
	if strings.EqualFold(adapter, "rest") {
		if params, ok := matchOperationPath(operation.Name, originalPath); ok {
			pathParams = params
		}
	}

	resolverRequest := cloneResolverRequest(ctx, request, operation, originalPath)
	return s.resolve(ctx, suite, surface, operation, pathParams, resolverRequest, adapter)
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
		return withOperationMetadata(errorResult(http.StatusBadRequest, "application/json", fmt.Sprintf(`{"error":"%s"}`, escapeJSONString("Could not read request body."))), operation, adapter), nil
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
		return withOperationMetadata(result, operation, adapter), nil
	}

	stateKey := ""
	if operation.MockMetadata.State != nil {
		stateKey = renderTemplate(operation.MockMetadata.State.LookupKeyTemplate, buildTemplateContext(*suite, surface, operation, snapshot, nil, nil, ""))
	}
	state := s.loadState(stateKey, operation.MockMetadata.State)
	schemaExamples := loadSchemaExamples(*suite, operation)
	contextMap := buildTemplateContext(*suite, surface, operation, snapshot, state, nil, "")

	schemaExample, matchedSchema, schemaValidation := matchSchemaExample(schemaExamples, snapshot)
	if schemaValidation != nil {
		return withOperationMetadata(schemaValidation, operation, adapter), nil
	}
	if matchedSchema {
		if operation.MockMetadata.DelayMillis > 0 {
			time.Sleep(time.Duration(operation.MockMetadata.DelayMillis) * time.Millisecond)
		}

		result := buildSchemaResult(*suite, surface, operation, snapshot, state, adapter, schemaExample)
		s.applyStateTransition(operation.MockMetadata.State, stateKey, schemaExample.Name, *suite, surface, operation, snapshot, state, result)
		return result, nil
	}

	example, ok := matchExample(operation, snapshot)
	if !ok {
		result, err := s.resolveFallback(ctx, *suite, surface, operation, snapshot, state, request, schemaExamples)
		return withOperationMetadata(result, operation, adapter), err
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
		Dispatcher:     firstNonEmpty(operation.Dispatcher, operation.MockMetadata.Dispatcher),
		ResolverURL:    operation.MockMetadata.ResolverURL,
		RuntimeURL:     operation.MockMetadata.RuntimeURL,
		MatchedExample: example.Name,
	}

	s.applyStateTransition(operation.MockMetadata.State, stateKey, example.Name, *suite, surface, operation, snapshot, state, result)
	return result, nil
}

func (s *Service) resolveFallback(ctx context.Context, suite suites.Definition, surface suites.APISurface, operation suites.APIOperation, snapshot requestSnapshot, state map[string]string, request *http.Request, schemaExamples []schemaBackedExample) (*Result, error) {
	fallback := operation.MockMetadata.Fallback
	if fallback == nil {
		return errorResult(http.StatusNotFound, "application/json", fmt.Sprintf(`{"error":"%s"}`, escapeJSONString("No matching mock exchange was found."))), nil
	}

	switch strings.ToLower(strings.TrimSpace(fallback.Mode)) {
	case "example":
		for _, example := range schemaExamples {
			if example.Name != fallback.ExampleName {
				continue
			}
			return buildSchemaResult(suite, surface, operation, snapshot, state, operation.MockMetadata.Adapter, example), nil
		}
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
				Dispatcher:     firstNonEmpty(operation.Dispatcher, operation.MockMetadata.Dispatcher),
				ResolverURL:    operation.MockMetadata.ResolverURL,
				RuntimeURL:     operation.MockMetadata.RuntimeURL,
				MatchedExample: example.Name,
			}, nil
		}
	case "proxy":
		return proxyFallback(ctx, request, fallback, firstNonEmpty(operation.Dispatcher, operation.MockMetadata.Dispatcher), operation.MockMetadata.ResolverURL, operation.MockMetadata.RuntimeURL)
	default:
		contextMap := buildTemplateContext(suite, surface, operation, snapshot, state, nil, "")
		headers := make(http.Header)
		for _, header := range fallback.Headers {
			headers.Set(header.Name, renderTemplate(header.Value, contextMap))
		}
		applyRecopiedConstraintHeaders(headers, operation.MockMetadata.ParameterConstraints, snapshot)
		return &Result{
			Status:      parseStatusCode(fallback.Status, http.StatusBadRequest),
			MediaType:   firstNonEmpty(fallback.MediaType, inferredMediaType(fallback.Body)),
			Headers:     headers,
			Body:        []byte(renderTemplate(fallback.Body, contextMap)),
			Adapter:     operation.MockMetadata.Adapter,
			Dispatcher:  firstNonEmpty(operation.Dispatcher, operation.MockMetadata.Dispatcher),
			ResolverURL: operation.MockMetadata.ResolverURL,
			RuntimeURL:  operation.MockMetadata.RuntimeURL,
		}, nil
	}

	return errorResult(http.StatusNotFound, "application/json", fmt.Sprintf(`{"error":"%s"}`, escapeJSONString("Fallback example was not found."))), nil
}

func proxyFallback(ctx context.Context, original *http.Request, fallback *suites.MockFallback, dispatcher, resolverURL, runtimeURL string) (*Result, error) {
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
		Status:      response.StatusCode,
		MediaType:   response.Header.Get("Content-Type"),
		Headers:     headers,
		Body:        responseBody,
		Adapter:     "proxy",
		Dispatcher:  dispatcher,
		ResolverURL: resolverURL,
		RuntimeURL:  runtimeURL,
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

func loadSchemaExamples(suite suites.Definition, operation suites.APIOperation) []schemaBackedExample {
	content, ok := sourceFileContent(suite.SourceFiles, operation.MockPath)
	if !ok {
		return nil
	}

	var document schemaExampleDocument
	if err := json.Unmarshal([]byte(content), &document); err != nil || len(document.Examples) == 0 {
		return nil
	}

	names := make([]string, 0, len(document.Examples))
	for name := range document.Examples {
		names = append(names, name)
	}
	sort.Strings(names)

	output := make([]schemaBackedExample, 0, len(names))
	for _, name := range names {
		example := document.Examples[name]
		output = append(output, schemaBackedExample{
			Name:     name,
			Dispatch: example.Dispatch,
			Request:  example.RequestSchema,
			Response: example.ResponseSchema,
		})
	}
	return output
}

func sourceFileContent(files []suites.SourceFile, path string) (string, bool) {
	for _, file := range files {
		if strings.TrimSpace(file.Path) == strings.TrimSpace(path) {
			return file.Content, true
		}
	}
	return "", false
}

func matchSchemaExample(examples []schemaBackedExample, snapshot requestSnapshot) (schemaBackedExample, bool, *Result) {
	if len(examples) == 1 && len(examples[0].Dispatch) == 0 {
		if result := validateSchemaRequest(examples[0].Request, snapshot); result != nil {
			return schemaBackedExample{}, false, result
		}
		return examples[0], true, nil
	}

	for _, example := range examples {
		if !matchWhen(example.Dispatch, snapshot) {
			continue
		}
		if result := validateSchemaRequest(example.Request, snapshot); result != nil {
			return schemaBackedExample{}, false, result
		}
		return example, true, nil
	}
	return schemaBackedExample{}, false, nil
}

func validateSchemaRequest(requestSchema schemaRequestSpec, snapshot requestSnapshot) *Result {
	if message := validateSchemaValue(requestSchema.Headers, snapshot.Headers, "request headers"); message != "" {
		return errorResult(http.StatusBadRequest, "application/json", fmt.Sprintf(`{"error":"%s"}`, escapeJSONString(message)))
	}
	if message := validateSchemaValue(requestSchema.Body, snapshot.BodyJSON, "request body"); message != "" {
		return errorResult(http.StatusBadRequest, "application/json", fmt.Sprintf(`{"error":"%s"}`, escapeJSONString(message)))
	}
	return nil
}

func validateSchemaValue(schema any, value any, location string) string {
	node, ok := schema.(map[string]any)
	if !ok || len(node) == 0 {
		return ""
	}

	schemaType := strings.TrimSpace(fmt.Sprint(node["type"]))
	switch schemaType {
	case "null":
		if value != nil {
			return fmt.Sprintf("%s must be empty.", strings.Title(location))
		}
	case "object":
		object, ok := toMapAny(value)
		if !ok {
			return fmt.Sprintf("%s must be an object.", strings.Title(location))
		}
		required := schemaStringList(node["required"])
		properties, _ := node["properties"].(map[string]any)
		for _, key := range required {
			actual, exists := object[key]
			if !exists || actual == nil || (strings.TrimSpace(fmt.Sprint(actual)) == "" && !isZeroBoolean(actual) && !isNumericValue(actual)) {
				return fmt.Sprintf("Missing required %s field %q.", location, key)
			}
		}
		for key, propertySchema := range properties {
			actual, exists := object[key]
			if !exists {
				continue
			}
			if message := validateSchemaValue(propertySchema, actual, location+"."+key); message != "" {
				return message
			}
		}
	case "array":
		items, ok := value.([]any)
		if !ok {
			if value == nil {
				return ""
			}
			return fmt.Sprintf("%s must be an array.", strings.Title(location))
		}
		itemSchema, _ := node["items"]
		for index, item := range items {
			if message := validateSchemaValue(itemSchema, item, fmt.Sprintf("%s[%d]", location, index)); message != "" {
				return message
			}
		}
	case "string":
		if value == nil {
			return ""
		}
		if _, ok := value.(string); !ok {
			return fmt.Sprintf("%s must be a string.", strings.Title(location))
		}
	case "integer":
		if value == nil {
			return ""
		}
		if !isIntegerValue(value) {
			return fmt.Sprintf("%s must be an integer.", strings.Title(location))
		}
	case "number":
		if value == nil {
			return ""
		}
		if !isNumericValue(value) {
			return fmt.Sprintf("%s must be a number.", strings.Title(location))
		}
	case "boolean":
		if value == nil {
			return ""
		}
		if _, ok := value.(bool); !ok {
			return fmt.Sprintf("%s must be a boolean.", strings.Title(location))
		}
	}

	return ""
}

func buildSchemaResult(suite suites.Definition, surface suites.APISurface, operation suites.APIOperation, snapshot requestSnapshot, state map[string]string, adapter string, example schemaBackedExample) *Result {
	contextMap := buildTemplateContext(suite, surface, operation, snapshot, state, nil, example.Name)
	headers := renderSchemaHeaders(example.Response.Headers, contextMap)
	applyRecopiedConstraintHeaders(headers, operation.MockMetadata.ParameterConstraints, snapshot)
	body := renderSchemaBody(example.Response.Body, example.Response.MediaType, contextMap)
	return &Result{
		Status:         parseStatusCode(example.Response.Status, http.StatusOK),
		MediaType:      firstNonEmpty(example.Response.MediaType, inferredMediaType(body)),
		Headers:        headers,
		Body:           []byte(body),
		Adapter:        adapter,
		Dispatcher:     firstNonEmpty(operation.Dispatcher, operation.MockMetadata.Dispatcher),
		ResolverURL:    operation.MockMetadata.ResolverURL,
		RuntimeURL:     operation.MockMetadata.RuntimeURL,
		MatchedExample: example.Name,
	}
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
			"id":          operation.ID,
			"name":        operation.Name,
			"resolverUrl": operation.MockMetadata.ResolverURL,
			"runtimeUrl":  operation.MockMetadata.RuntimeURL,
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

	return templateTokenPattern.ReplaceAllStringFunc(input, func(token string) string {
		match := templateTokenPattern.FindStringSubmatch(token)
		if len(match) != 2 {
			return token
		}
		value, ok := evaluateTemplateExpression(match[1], contextMap)
		if !ok {
			return ""
		}
		return value
	})
}

func evaluateTemplateExpression(expression string, contextMap map[string]any) (string, bool) {
	options := splitTemplateExpression(expression, "||")
	if len(options) > 1 {
		for _, option := range options {
			value, ok := evaluateTemplateExpression(option, contextMap)
			if ok && strings.TrimSpace(value) != "" {
				return value, true
			}
		}
		return "", false
	}

	trimmed := strings.TrimSpace(expression)
	if trimmed == "" {
		return "", false
	}

	if literal, ok := unquoteTemplateLiteral(trimmed); ok {
		return literal, true
	}

	if match := functionCallPattern.FindStringSubmatch(trimmed); len(match) == 3 {
		return evaluateTemplateFunction(match[1], splitTemplateArguments(match[2]), contextMap)
	}

	value, ok := lookupContextValue(contextMap, templatePathSegments(trimmed))
	if !ok {
		return "", false
	}
	return fmt.Sprint(value), true
}

func lookupContextValue(value any, segments []string) (any, bool) {
	current := value
	for _, segment := range segments {
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

func unquoteTemplateLiteral(value string) (string, bool) {
	if len(value) < 2 {
		return "", false
	}
	if (value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"') {
		unquoted, err := strconv.Unquote(`"` + strings.ReplaceAll(value[1:len(value)-1], `"`, `\"`) + `"`)
		if err != nil {
			return value[1 : len(value)-1], true
		}
		return unquoted, true
	}
	return "", false
}

func evaluateTemplateFunction(name string, args []string, contextMap map[string]any) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "now", "timestamp":
		layout := time.RFC3339Nano
		if len(args) > 0 {
			layout = timeLayoutForTemplateArg(argumentValue(args[0], contextMap))
		}
		return time.Now().UTC().Format(layout), true
	case "uuid", "guid", "randomuuid":
		return uuid.NewString(), true
	case "randomint":
		return strconv.Itoa(randomInt(args, contextMap)), true
	case "randomstring":
		length := max(argumentInt(args, 0, contextMap, 12), 1)
		return randomString(length), true
	case "randomboolean":
		if randomInt(nil, nil)%2 == 0 {
			return "false", true
		}
		return "true", true
	case "randomvalue":
		if len(args) == 0 {
			return "", false
		}
		index := randomInt([]string{"0", strconv.Itoa(len(args) - 1)}, nil)
		return argumentValue(args[index], contextMap), true
	default:
		return "", false
	}
}

func timeLayoutForTemplateArg(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.RFC3339Nano
	}

	replacements := []struct {
		from string
		to   string
	}{
		{"yyyy", "2006"},
		{"MM", "01"},
		{"dd", "02"},
		{"HH", "15"},
		{"mm", "04"},
		{"ss", "05"},
	}

	layout := trimmed
	for _, replacement := range replacements {
		layout = strings.ReplaceAll(layout, replacement.from, replacement.to)
	}
	return layout
}

func randomInt(args []string, contextMap map[string]any) int {
	minimum := 0
	maximum := 2147483647

	switch len(args) {
	case 1:
		maximum = argumentInt(args, 0, contextMap, maximum)
	case 2:
		minimum = argumentInt(args, 0, contextMap, minimum)
		maximum = argumentInt(args, 1, contextMap, maximum)
	}

	if maximum < minimum {
		minimum, maximum = maximum, minimum
	}
	if maximum == minimum {
		return minimum
	}

	span := int64(maximum-minimum) + 1
	value, err := cryptoRand.Int(cryptoRand.Reader, big.NewInt(span))
	if err != nil {
		return minimum
	}
	return minimum + int(value.Int64())
}

func randomString(length int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	builder := strings.Builder{}
	builder.Grow(length)
	for index := 0; index < length; index++ {
		position, err := cryptoRand.Int(cryptoRand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			builder.WriteByte(alphabet[index%len(alphabet)])
			continue
		}
		builder.WriteByte(alphabet[position.Int64()])
	}
	return builder.String()
}

func argumentInt(args []string, index int, contextMap map[string]any, fallback int) int {
	if index >= len(args) {
		return fallback
	}
	value := argumentValue(args[index], contextMap)
	if parsed, err := strconv.Atoi(value); err == nil {
		return parsed
	}
	return fallback
}

func argumentValue(argument string, contextMap map[string]any) string {
	if literal, ok := unquoteTemplateLiteral(strings.TrimSpace(argument)); ok {
		return literal
	}
	if value, ok := evaluateTemplateExpression(argument, contextMap); ok {
		return value
	}
	return strings.TrimSpace(argument)
}

func splitTemplateArguments(input string) []string {
	return splitTemplateExpression(input, ",")
}

func splitTemplateExpression(input, delimiter string) []string {
	if strings.TrimSpace(input) == "" {
		return nil
	}

	parts := make([]string, 0)
	depth := 0
	inSingleQuote := false
	inDoubleQuote := false
	start := 0

	for index := 0; index < len(input); index++ {
		char := input[index]
		switch char {
		case '\'':
			if !inDoubleQuote {
				inSingleQuote = !inSingleQuote
			}
		case '"':
			if !inSingleQuote {
				inDoubleQuote = !inDoubleQuote
			}
		case '(':
			if !inSingleQuote && !inDoubleQuote {
				depth++
			}
		case ')':
			if !inSingleQuote && !inDoubleQuote && depth > 0 {
				depth--
			}
		}

		if inSingleQuote || inDoubleQuote || depth > 0 {
			continue
		}
		if strings.HasPrefix(input[index:], delimiter) {
			parts = append(parts, strings.TrimSpace(input[start:index]))
			index += len(delimiter) - 1
			start = index + 1
		}
	}

	parts = append(parts, strings.TrimSpace(input[start:]))
	return compactTemplateParts(parts)
}

func compactTemplateParts(input []string) []string {
	output := make([]string, 0, len(input))
	for _, value := range input {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			output = append(output, trimmed)
		}
	}
	return output
}

func templatePathSegments(path string) []string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil
	}

	segments := make([]string, 0)
	buffer := strings.Builder{}
	for index := 0; index < len(trimmed); index++ {
		char := trimmed[index]
		switch char {
		case '.', '/':
			if buffer.Len() > 0 {
				segments = append(segments, buffer.String())
				buffer.Reset()
			}
		case '[':
			if buffer.Len() > 0 {
				segments = append(segments, buffer.String())
				buffer.Reset()
			}
			end := strings.IndexByte(trimmed[index+1:], ']')
			if end < 0 {
				continue
			}
			key := strings.TrimSpace(trimmed[index+1 : index+1+end])
			key = strings.Trim(key, `"'`)
			if key != "" {
				segments = append(segments, key)
			}
			index += end + 1
		default:
			buffer.WriteByte(char)
		}
	}
	if buffer.Len() > 0 {
		segments = append(segments, buffer.String())
	}
	return compactTemplateParts(segments)
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

func renderSchemaHeaders(schema any, contextMap map[string]any) http.Header {
	headers := make(http.Header)
	values, ok := renderSchemaValue(schema, contextMap).(map[string]any)
	if !ok {
		return headers
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if values[key] == nil {
			continue
		}
		headers.Set(key, fmt.Sprint(values[key]))
	}
	return headers
}

func renderSchemaBody(schema any, mediaType string, contextMap map[string]any) string {
	rendered := renderSchemaValue(schema, contextMap)
	if rendered == nil {
		return ""
	}

	normalizedMediaType := strings.ToLower(strings.TrimSpace(mediaType))
	switch rendered.(type) {
	case map[string]any, []any:
		body, err := json.MarshalIndent(rendered, "", "  ")
		if err == nil {
			return string(body)
		}
	}
	if normalizedMediaType == "" {
		normalizedMediaType = inferredMediaType(fmt.Sprint(rendered))
	}
	if strings.Contains(normalizedMediaType, "json") {
		body, err := json.MarshalIndent(rendered, "", "  ")
		if err == nil {
			return string(body)
		}
	}
	return fmt.Sprint(rendered)
}

func renderSchemaValue(schema any, contextMap map[string]any) any {
	node, ok := schema.(map[string]any)
	if !ok {
		return schema
	}

	if template, ok := node["x-babel-template"].(string); ok {
		return coerceSchemaPrimitive(strings.TrimSpace(fmt.Sprint(node["type"])), renderTemplate(template, contextMap))
	}
	if example, exists := node["example"]; exists {
		return example
	}

	switch strings.TrimSpace(fmt.Sprint(node["type"])) {
	case "object":
		properties, _ := node["properties"].(map[string]any)
		if len(properties) == 0 {
			return map[string]any{}
		}
		keys := make([]string, 0, len(properties))
		for key := range properties {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		result := make(map[string]any, len(keys))
		for _, key := range keys {
			result[key] = renderSchemaValue(properties[key], contextMap)
		}
		return result
	case "array":
		if items, exists := node["items"]; exists {
			return []any{renderSchemaValue(items, contextMap)}
		}
		return []any{}
	case "integer":
		return int64(0)
	case "number":
		return 0.0
	case "boolean":
		return false
	case "null":
		return nil
	default:
		return ""
	}
}

func coerceSchemaPrimitive(schemaType, value string) any {
	switch schemaType {
	case "integer":
		if parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil {
			return parsed
		}
	case "number":
		if parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
			return parsed
		}
	case "boolean":
		if parsed, err := strconv.ParseBool(strings.TrimSpace(value)); err == nil {
			return parsed
		}
	case "null":
		return nil
	}
	return value
}

func toMapAny(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return typed, true
	case map[string]string:
		output := make(map[string]any, len(typed))
		for key, entry := range typed {
			output[key] = entry
		}
		return output, true
	default:
		return nil, false
	}
}

func schemaStringList(value any) []string {
	input, ok := value.([]any)
	if !ok {
		return nil
	}
	output := make([]string, 0, len(input))
	for _, item := range input {
		trimmed := strings.TrimSpace(fmt.Sprint(item))
		if trimmed != "" {
			output = append(output, trimmed)
		}
	}
	return output
}

func isNumericValue(value any) bool {
	switch value.(type) {
	case int, int8, int16, int32, int64, float32, float64, uint, uint8, uint16, uint32, uint64:
		return true
	default:
		return false
	}
}

func isIntegerValue(value any) bool {
	switch typed := value.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return true
	case float64:
		return typed == float64(int64(typed))
	case float32:
		return typed == float32(int64(typed))
	default:
		return false
	}
}

func isZeroBoolean(value any) bool {
	typed, ok := value.(bool)
	return ok && !typed
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
