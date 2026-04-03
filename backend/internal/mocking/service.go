package mocking

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/babelsuite/babelsuite/internal/suites"
)

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
