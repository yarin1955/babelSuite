package mocking

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/babelsuite/babelsuite/internal/suites"
)

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
