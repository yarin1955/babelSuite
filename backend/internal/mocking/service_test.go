package mocking

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/babelsuite/babelsuite/internal/suites"
	"github.com/google/uuid"
)

type staticSuiteReader struct {
	suite suites.Definition
}

func (r staticSuiteReader) Get(id string) (*suites.Definition, error) {
	if id != r.suite.ID {
		return nil, suites.ErrNotFound
	}
	clone := r.suite
	return &clone, nil
}

func TestPaymentMockPersistsStateAcrossOperations(t *testing.T) {
	service := NewService(suites.NewService())

	createReq := httptest.NewRequest("POST", "/mocks/rest/payment-suite/payment-gateway/payments?status=approved", strings.NewReader(`{"amount":1299,"currency":"USD","merchantId":"m-117"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("X-Suite-Profile", "year.yaml")

	createResult, err := service.InvokeREST(context.Background(), "payment-suite", "payment-gateway", "/payments", createReq)
	if err != nil {
		t.Fatalf("InvokeREST(create): %v", err)
	}
	if createResult.Status != 201 {
		t.Fatalf("expected create status 201, got %d", createResult.Status)
	}

	getReq := httptest.NewRequest("GET", "/mocks/rest/payment-suite/payment-gateway/payments/pay_1043", nil)
	getReq.Header.Set("X-Request-Id", "req-100")

	getResult, err := service.InvokeREST(context.Background(), "payment-suite", "payment-gateway", "/payments/pay_1043", getReq)
	if err != nil {
		t.Fatalf("InvokeREST(get): %v", err)
	}
	if getResult.Status != 200 {
		t.Fatalf("expected get status 200, got %d", getResult.Status)
	}
	body := string(getResult.Body)
	if !strings.Contains(body, `"profile": "year.yaml"`) {
		t.Fatalf("expected stateful profile in response, got %s", body)
	}
	if !strings.Contains(body, `"status": "authorized"`) {
		t.Fatalf("expected authorized status in response, got %s", body)
	}
	if got := getResult.Headers.Get("X-Request-Id"); got != "req-100" {
		t.Fatalf("expected recopied X-Request-Id header, got %q", got)
	}
}

func TestResetSuiteStateClearsPersistedMockState(t *testing.T) {
	service := NewService(suites.NewService())

	createReq := httptest.NewRequest("POST", "/mocks/rest/payment-suite/payment-gateway/payments?status=approved", strings.NewReader(`{"amount":1299,"currency":"USD","merchantId":"m-117"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("X-Suite-Profile", "year.yaml")

	createResult, err := service.InvokeREST(context.Background(), "payment-suite", "payment-gateway", "/payments", createReq)
	if err != nil {
		t.Fatalf("InvokeREST(create): %v", err)
	}
	if createResult.Status != 201 {
		t.Fatalf("expected create status 201, got %d", createResult.Status)
	}

	if err := service.ResetSuiteState(context.Background(), "payment-suite"); err != nil {
		t.Fatalf("ResetSuiteState: %v", err)
	}

	getReq := httptest.NewRequest("GET", "/mocks/rest/payment-suite/payment-gateway/payments/pay_1043", nil)
	getResult, err := service.InvokeREST(context.Background(), "payment-suite", "payment-gateway", "/payments/pay_1043", getReq)
	if err != nil {
		t.Fatalf("InvokeREST(get): %v", err)
	}
	body := string(getResult.Body)
	if strings.Contains(body, `"profile": "year.yaml"`) {
		t.Fatalf("expected reset suite state to remove persisted profile, got %s", body)
	}
}

func TestListProductsEnforcesConstraints(t *testing.T) {
	service := NewService(suites.NewService())

	req := httptest.NewRequest("GET", "/mocks/rest/storefront-browser-lab/storefront-api/catalog/products", nil)
	req.Header.Set("Accept", "application/json")

	result, err := service.InvokeREST(context.Background(), "storefront-browser-lab", "storefront-api", "/catalog/products", req)
	if err != nil {
		t.Fatalf("InvokeREST: %v", err)
	}
	if result.Status != 400 {
		t.Fatalf("expected status 400, got %d", result.Status)
	}
	if !strings.Contains(string(result.Body), "Missing required query parameter") {
		t.Fatalf("expected missing query error, got %s", string(result.Body))
	}
}

func TestGRPCAdapterUsesBodyDispatch(t *testing.T) {
	service := NewService(suites.NewService())

	req := httptest.NewRequest("POST", "/mocks/grpc/fleet-control-room/dispatcher-api/assign-route", strings.NewReader(`{"vehicleId":"vh-11","routeId":"route-778"}`))
	req.Header.Set("X-Profile", "perf.yaml")

	result, err := service.InvokeAdapter(context.Background(), "fleet-control-room", "dispatcher-api", "assign-route", "grpc", req)
	if err != nil {
		t.Fatalf("InvokeAdapter: %v", err)
	}
	if result.Status != 200 {
		t.Fatalf("expected status 200, got %d", result.Status)
	}
	if !strings.Contains(string(result.Body), `"assignmentId": "asg-778"`) {
		t.Fatalf("expected grpc mock body, got %s", string(result.Body))
	}
	if got := result.Headers.Get("x-profile"); got != "perf.yaml" {
		t.Fatalf("expected recopied x-profile header, got %q", got)
	}
}

func TestRenderTemplateResolvesContextPathsAndFallbacks(t *testing.T) {
	rendered := renderTemplate(`{
  "servedBy": "{{ request.headers.x-mock-node || 'apisix-sidecar' }}",
  "returnId": "{{ request.path.returnId }}",
  "status": "{{ state.status }}"
}`, map[string]any{
		"request": map[string]any{
			"headers": map[string]string{},
			"path": map[string]string{
				"returnId": "ret_9001",
			},
		},
		"state": map[string]string{
			"status": "approved",
		},
	})

	var payload map[string]string
	if err := json.Unmarshal([]byte(rendered), &payload); err != nil {
		t.Fatalf("unmarshal rendered template: %v", err)
	}
	if payload["servedBy"] != "apisix-sidecar" {
		t.Fatalf("expected fallback value, got %q", payload["servedBy"])
	}
	if payload["returnId"] != "ret_9001" {
		t.Fatalf("expected request path value, got %q", payload["returnId"])
	}
	if payload["status"] != "approved" {
		t.Fatalf("expected state value, got %q", payload["status"])
	}
}

func TestReturnsMocksGenerateFreshDataPerRequest(t *testing.T) {
	service := NewService(suites.NewService())

	createReq := httptest.NewRequest("POST", "/mocks/rest/returns-control-plane/returns-api/returns?scenario=approved", strings.NewReader(`{"orderId":"ord_1001","returnReason":"damaged","amountCents":4200}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("X-Suite-Profile", "canary.yaml")

	createResult, err := service.InvokeREST(context.Background(), "returns-control-plane", "returns-api", "/returns", createReq)
	if err != nil {
		t.Fatalf("InvokeREST(create): %v", err)
	}
	if createResult.Status != 201 {
		t.Fatalf("expected create status 201, got %d", createResult.Status)
	}

	var createPayload map[string]string
	if err := json.Unmarshal(createResult.Body, &createPayload); err != nil {
		t.Fatalf("unmarshal create payload: %v", err)
	}
	returnID := createPayload["returnId"]
	if matched, _ := regexp.MatchString(`^ret_[0-9]+$`, returnID); !matched {
		t.Fatalf("expected generated return id, got %q", returnID)
	}
	if _, err := uuid.Parse(createPayload["traceId"]); err != nil {
		t.Fatalf("expected create trace id, got %q", createPayload["traceId"])
	}

	getReqOne := httptest.NewRequest("GET", "/mocks/rest/returns-control-plane/returns-api/returns/"+returnID, nil)
	getReqOne.Header.Set("X-Request-Id", "req-201")
	getResultOne, err := service.InvokeREST(context.Background(), "returns-control-plane", "returns-api", "/returns/"+returnID, getReqOne)
	if err != nil {
		t.Fatalf("InvokeREST(get #1): %v", err)
	}

	getReqTwo := httptest.NewRequest("GET", "/mocks/rest/returns-control-plane/returns-api/returns/"+returnID, nil)
	getReqTwo.Header.Set("X-Request-Id", "req-202")
	getResultTwo, err := service.InvokeREST(context.Background(), "returns-control-plane", "returns-api", "/returns/"+returnID, getReqTwo)
	if err != nil {
		t.Fatalf("InvokeREST(get #2): %v", err)
	}

	var getPayloadOne map[string]string
	if err := json.Unmarshal(getResultOne.Body, &getPayloadOne); err != nil {
		t.Fatalf("unmarshal get #1 payload: %v", err)
	}
	var getPayloadTwo map[string]string
	if err := json.Unmarshal(getResultTwo.Body, &getPayloadTwo); err != nil {
		t.Fatalf("unmarshal get #2 payload: %v", err)
	}

	if getPayloadOne["returnId"] != returnID {
		t.Fatalf("expected state lookup for %q, got %q", returnID, getPayloadOne["returnId"])
	}
	if getPayloadOne["profile"] != "canary.yaml" {
		t.Fatalf("expected stored suite profile, got %q", getPayloadOne["profile"])
	}
	if getPayloadOne["servedBy"] != "apisix-sidecar" {
		t.Fatalf("expected fallback servedBy, got %q", getPayloadOne["servedBy"])
	}
	if getPayloadOne["traceId"] == getPayloadTwo["traceId"] {
		t.Fatalf("expected a fresh trace id per response, got %q", getPayloadOne["traceId"])
	}
	if _, err := uuid.Parse(getPayloadOne["traceId"]); err != nil {
		t.Fatalf("expected get trace id, got %q", getPayloadOne["traceId"])
	}
	if _, err := time.Parse(time.RFC3339Nano, getPayloadOne["servedAt"]); err != nil {
		t.Fatalf("expected get timestamp, got %q", getPayloadOne["servedAt"])
	}
	if got := getResultOne.Headers.Get("X-Request-Id"); got != "req-201" {
		t.Fatalf("expected recopied X-Request-Id header, got %q", got)
	}
}

func TestSOAPMockRendersXMLResponses(t *testing.T) {
	service := NewService(suites.NewService())

	req := httptest.NewRequest("POST", "/mocks/rest/soap-claims-hub/claims-soap/ClaimService", strings.NewReader(`<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:clm="urn:claims:v1">
  <soapenv:Header/>
  <soapenv:Body>
    <clm:SubmitClaimRequest>
      <clm:PolicyNumber>PL-1024</clm:PolicyNumber>
      <clm:LossType>water</clm:LossType>
      <clm:Amount>4200</clm:Amount>
    </clm:SubmitClaimRequest>
  </soapenv:Body>
</soapenv:Envelope>`))
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", "urn:SubmitClaim")
	req.Header.Set("X-Suite-Profile", "canary.yaml")

	result, err := service.InvokeREST(context.Background(), "soap-claims-hub", "claims-soap", "/ClaimService", req)
	if err != nil {
		t.Fatalf("InvokeREST(soap): %v", err)
	}
	if result.Status != 200 {
		t.Fatalf("expected status 200, got %d", result.Status)
	}
	if !strings.Contains(strings.ToLower(result.MediaType), "xml") {
		t.Fatalf("expected xml media type, got %q", result.MediaType)
	}

	body := string(result.Body)
	if strings.Contains(body, "{{") {
		t.Fatalf("expected rendered soap body, got %s", body)
	}
	if !strings.Contains(body, "<clm:Decision>APPROVED</clm:Decision>") {
		t.Fatalf("expected submit claim decision in xml body, got %s", body)
	}
	if !strings.Contains(body, "<clm:Profile>canary.yaml</clm:Profile>") {
		t.Fatalf("expected forwarded suite profile in xml body, got %s", body)
	}
	if matched, _ := regexp.MatchString(`<clm:ClaimId>clm_[0-9]+</clm:ClaimId>`, body); !matched {
		t.Fatalf("expected generated claim id in xml body, got %s", body)
	}
	if matched, _ := regexp.MatchString(`<clm:TraceId>[0-9a-fA-F-]{36}</clm:TraceId>`, body); !matched {
		t.Fatalf("expected generated trace id in xml body, got %s", body)
	}
	if got := result.Headers.Get("x-suite-profile"); got != "canary.yaml" {
		t.Fatalf("expected recopied x-suite-profile header, got %q", got)
	}
}

func TestResolverEndpointReturnsNormalizedEnvelope(t *testing.T) {
	service := NewService(suites.NewService())
	handler := NewHandler(service, "")
	mux := http.NewServeMux()
	handler.Register(mux)

	req := httptest.NewRequest("POST", "/internal/mock-data/returns-control-plane/returns-api/create-return?scenario=approved", strings.NewReader(`{"orderId":"ord_1001","returnReason":"damaged","amountCents":4200}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Babelsuite-Original-Method", "POST")
	req.Header.Set("X-Babelsuite-Original-Path", "/returns")
	req.Header.Set("X-Suite-Profile", "canary.yaml")

	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected resolver status 200, got %d", recorder.Code)
	}

	var envelope struct {
		Status         int                 `json:"status"`
		MediaType      string              `json:"mediaType"`
		Headers        map[string][]string `json:"headers"`
		Body           string              `json:"body"`
		Adapter        string              `json:"adapter"`
		Dispatcher     string              `json:"dispatcher"`
		ResolverURL    string              `json:"resolverUrl"`
		RuntimeURL     string              `json:"runtimeUrl"`
		MatchedExample string              `json:"matchedExample"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal resolver envelope: %v", err)
	}
	if envelope.Status != 201 {
		t.Fatalf("expected wrapped response status 201, got %d", envelope.Status)
	}
	if envelope.Adapter != "rest" {
		t.Fatalf("expected resolver adapter rest, got %q", envelope.Adapter)
	}
	if envelope.Dispatcher != "apisix" {
		t.Fatalf("expected resolver dispatcher apisix, got %q", envelope.Dispatcher)
	}
	if envelope.ResolverURL != "/internal/mock-data/returns-control-plane/returns-api/create-return" {
		t.Fatalf("expected resolver url in envelope, got %q", envelope.ResolverURL)
	}
	if envelope.RuntimeURL != "/mocks/rest/returns-control-plane/returns-api/returns?scenario=approved" {
		t.Fatalf("expected compatibility runtime url in envelope, got %q", envelope.RuntimeURL)
	}
	if envelope.MatchedExample != "approved-standard" {
		t.Fatalf("expected matched example approved-standard, got %q", envelope.MatchedExample)
	}
	if strings.Contains(envelope.Body, "{{") {
		t.Fatalf("expected rendered resolver body, got %s", envelope.Body)
	}
}

func TestResolverEndpointPreservesPathParamsForRESTOperations(t *testing.T) {
	service := NewService(suites.NewService())
	createReq := httptest.NewRequest("POST", "/mocks/rest/returns-control-plane/returns-api/returns?scenario=approved", strings.NewReader(`{"orderId":"ord_1001","returnReason":"damaged","amountCents":4200}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("X-Suite-Profile", "canary.yaml")

	createResult, err := service.InvokeREST(context.Background(), "returns-control-plane", "returns-api", "/returns", createReq)
	if err != nil {
		t.Fatalf("InvokeREST(create): %v", err)
	}

	var created map[string]string
	if err := json.Unmarshal(createResult.Body, &created); err != nil {
		t.Fatalf("unmarshal create payload: %v", err)
	}
	returnID := created["returnId"]
	if returnID == "" {
		t.Fatal("expected generated return id")
	}

	handler := NewHandler(service, "")
	mux := http.NewServeMux()
	handler.Register(mux)

	req := httptest.NewRequest("POST", "/internal/mock-data/returns-control-plane/returns-api/get-return", nil)
	req.Header.Set("X-Babelsuite-Original-Method", "GET")
	req.Header.Set("X-Babelsuite-Original-Path", "/returns/"+returnID)
	req.Header.Set("X-Request-Id", "req-777")

	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected resolver status 200, got %d", recorder.Code)
	}

	var envelope struct {
		Status int    `json:"status"`
		Body   string `json:"body"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal resolver envelope: %v", err)
	}
	if envelope.Status != 200 {
		t.Fatalf("expected wrapped response status 200, got %d", envelope.Status)
	}
	if !strings.Contains(envelope.Body, `"returnId": "`+returnID+`"`) {
		t.Fatalf("expected resolver body to include path-derived return id %q, got %s", returnID, envelope.Body)
	}
}

func TestSchemaDocumentDrivesMockResponseGeneration(t *testing.T) {
	source := suites.NewService()
	suite, err := source.Get("returns-control-plane")
	if err != nil {
		t.Fatalf("get suite: %v", err)
	}

	replaceSuiteSourceFile(t, suite, "mock/events/refund-authorized.cue", `{
  "$schema": "https://schemas.babelsuite.dev/mock-exchange-source-v1.json",
  "examples": {
    "authorized-event": {
      "dispatch": [
        { "from": "query", "param": "type", "value": "returns.refund.authorized" }
      ],
      "requestSchema": {
        "headers": {
          "type": "object",
          "properties": {
            "content-type": { "type": "string", "example": "application/json" }
          },
          "required": ["content-type"]
        },
        "body": {
          "type": "object",
          "properties": {
            "returnId": { "type": "string", "example": "ret_1001" }
          },
          "required": ["returnId"]
        }
      },
      "responseSchema": {
        "status": "202",
        "mediaType": "application/json",
        "headers": {
          "type": "object",
          "properties": {
            "x-event-stream": { "type": "string", "example": "schema-driven" }
          },
          "required": ["x-event-stream"]
        },
        "body": {
          "type": "object",
          "properties": {
            "eventId": { "type": "string", "example": "evt_schema" },
            "returnId": { "type": "string", "x-babel-template": "{{ request.body.returnId }}" },
            "status": { "type": "string", "example": "published_from_schema" }
          },
          "required": ["eventId", "returnId", "status"]
        }
      }
    }
  }
}`)

	service := NewService(staticSuiteReader{suite: *suite})
	req := httptest.NewRequest("POST", "/mocks/async/returns-control-plane/refund-events/publish-refund-authorized?type=returns.refund.authorized", strings.NewReader(`{"returnId":"ret_9090"}`))
	req.Header.Set("Content-Type", "application/json")

	result, err := service.InvokeAdapter(context.Background(), "returns-control-plane", "refund-events", "publish-refund-authorized", "async", req)
	if err != nil {
		t.Fatalf("InvokeAdapter: %v", err)
	}
	if result.Status != 202 {
		t.Fatalf("expected status 202, got %d", result.Status)
	}
	if result.Headers.Get("x-event-stream") != "schema-driven" {
		t.Fatalf("expected schema-driven header, got %q", result.Headers.Get("x-event-stream"))
	}
	body := string(result.Body)
	if !strings.Contains(body, `"eventId": "evt_schema"`) {
		t.Fatalf("expected eventId from schema source file, got %s", body)
	}
	if !strings.Contains(body, `"returnId": "ret_9090"`) {
		t.Fatalf("expected templated request returnId, got %s", body)
	}
	if strings.Contains(body, `"type": "returns.refund.authorized"`) {
		t.Fatalf("expected runtime to ignore legacy hard-coded response body, got %s", body)
	}
}

func TestSchemaDocumentValidatesRequestBody(t *testing.T) {
	source := suites.NewService()
	suite, err := source.Get("returns-control-plane")
	if err != nil {
		t.Fatalf("get suite: %v", err)
	}

	replaceSuiteSourceFile(t, suite, "mock/events/refund-authorized.cue", `{
  "$schema": "https://schemas.babelsuite.dev/mock-exchange-source-v1.json",
  "examples": {
    "authorized-event": {
      "dispatch": [
        { "from": "query", "param": "type", "value": "returns.refund.authorized" }
      ],
      "requestSchema": {
        "body": {
          "type": "object",
          "properties": {
            "returnId": { "type": "string", "example": "ret_1001" }
          },
          "required": ["returnId"]
        }
      },
      "responseSchema": {
        "status": "202",
        "mediaType": "application/json",
        "body": {
          "type": "object",
          "properties": {
            "status": { "type": "string", "example": "ok" }
          },
          "required": ["status"]
        }
      }
    }
  }
}`)

	service := NewService(staticSuiteReader{suite: *suite})
	req := httptest.NewRequest("POST", "/mocks/async/returns-control-plane/refund-events/publish-refund-authorized?type=returns.refund.authorized", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")

	result, err := service.InvokeAdapter(context.Background(), "returns-control-plane", "refund-events", "publish-refund-authorized", "async", req)
	if err != nil {
		t.Fatalf("InvokeAdapter: %v", err)
	}
	if result.Status != 400 {
		t.Fatalf("expected status 400, got %d", result.Status)
	}
	if !strings.Contains(string(result.Body), `returnId`) {
		t.Fatalf("expected schema validation error, got %s", string(result.Body))
	}
}

func replaceSuiteSourceFile(t *testing.T, suite *suites.Definition, path, content string) {
	t.Helper()

	for index, file := range suite.SourceFiles {
		if file.Path != path {
			continue
		}
		suite.SourceFiles[index].Content = content
		return
	}

	t.Fatalf("suite source file %s not found", path)
}
