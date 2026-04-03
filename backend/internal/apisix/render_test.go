package apisix

import (
	"strings"
	"testing"
)

func TestRenderStandaloneConfigIncludesRESTRoutes(t *testing.T) {
	body := RenderStandaloneConfig(SuiteConfig{
		ID: "payment-suite",
		APISurfaces: []SurfaceConfig{
			{
				ID:       "payment-gateway",
				Protocol: "REST",
				MockHost: "https://payment-suite.mock.internal",
				Operations: []OperationConfig{
					{
						ID:      "create-payment",
						Method:  "POST",
						Name:    "/payments",
						Summary: "Create a payment authorization and fan out to Stripe.",
						MockMetadata: OperationMetadataConfig{
							Adapter:     "rest",
							ResolverURL: "/internal/mock-data/payment-suite/payment-gateway/create-payment",
							RuntimeURL:  "/mocks/rest/payment-suite/payment-gateway/payments?status=approved",
						},
					},
				},
			},
		},
	})

	if !strings.Contains(body, "uri: /payments") {
		t.Fatalf("expected payment route in APISIX config, got:\n%s", body)
	}
	if !strings.Contains(body, "/mocks/rest/payment-suite/payment-gateway/payments") {
		t.Fatalf("expected rewritten runtime path in APISIX config, got:\n%s", body)
	}
	if !strings.Contains(body, "name: ext-plugin-pre-req") {
		t.Fatalf("expected plugin catalog to include ext-plugin-pre-req, got:\n%s", body)
	}
	if !strings.Contains(body, "ext-plugin-pre-req:") {
		t.Fatalf("expected REST route to include ext-plugin-pre-req, got:\n%s", body)
	}
	if !strings.Contains(body, "resolver=/internal/mock-data/payment-suite/payment-gateway/create-payment") {
		t.Fatalf("expected resolver contract note in APISIX config, got:\n%s", body)
	}
	if !strings.Contains(body, "X-Babelsuite-Dispatcher: apisix") {
		t.Fatalf("expected dispatcher header in APISIX config, got:\n%s", body)
	}
	if !strings.HasSuffix(body, "#END\n") {
		t.Fatalf("expected APISIX standalone config terminator, got:\n%s", body)
	}
}

func TestRenderStandaloneConfigNotesDeferredAdapters(t *testing.T) {
	body := RenderStandaloneConfig(SuiteConfig{
		ID: "returns-control-plane",
		APISurfaces: []SurfaceConfig{
			{
				ID:       "pricing-service",
				Protocol: "gRPC",
				Operations: []OperationConfig{
					{
						ID:     "quote-refund",
						Method: "RPC",
						Name:   "/returns.v1.RefundPricing/QuoteRefund",
						MockMetadata: OperationMetadataConfig{
							Adapter:         "grpc",
							ResolverURL:     "/internal/mock-data/returns-control-plane/pricing-service/quote-refund",
							RuntimeURL:      "/mocks/grpc/returns-control-plane/pricing-service/quote-refund",
							DispatcherRules: "The APISIX sidecar should terminate or normalize gRPC traffic for /returns.v1.RefundPricing/QuoteRefund and call /internal/mock-data/returns-control-plane/pricing-service/quote-refund to fetch normalized status, headers, and body so APISIX remains the visible responder while BabelSuite only resolves the matched exchange.",
						},
					},
				},
			},
			{
				ID:       "refund-events",
				Protocol: "Async",
				MockHost: "kafka://returns-control-plane.mock.internal",
				Operations: []OperationConfig{
					{
						ID:      "publish-refund-authorized",
						Method:  "EVENT",
						Name:    "/returns.refund.authorized",
						MockURL: "kafka://returns-control-plane.mock.internal/returns.refund.authorized",
						MockMetadata: OperationMetadataConfig{
							Adapter:         "async",
							ResolverURL:     "/internal/mock-data/returns-control-plane/refund-events/publish-refund-authorized",
							RuntimeURL:      "/mocks/async/returns-control-plane/refund-events/publish-refund-authorized",
							DispatcherRules: "The APISIX sidecar should use a plugin-backed event adapter for /returns.refund.authorized and call /internal/mock-data/returns-control-plane/refund-events/publish-refund-authorized to fetch normalized payload data so APISIX owns the broker-facing response flow while BabelSuite only generates the mock payload.",
						},
					},
				},
			},
		},
	})

	if !strings.Contains(body, "returns-control-plane.quote-refund (gRPC)") {
		t.Fatalf("expected deferred gRPC adapter note, got:\n%s", body)
	}
	if !strings.Contains(body, "name: grpc-transcode") {
		t.Fatalf("expected plugin catalog to include grpc-transcode, got:\n%s", body)
	}
	if !strings.Contains(body, "name: kafka-proxy") {
		t.Fatalf("expected plugin catalog to include kafka-proxy, got:\n%s", body)
	}
	if !strings.Contains(body, "resolver=/internal/mock-data/returns-control-plane/pricing-service/quote-refund") {
		t.Fatalf("expected deferred gRPC resolver note, got:\n%s", body)
	}
	if !strings.Contains(body, "returns-control-plane.publish-refund-authorized (Async)") {
		t.Fatalf("expected deferred async adapter note, got:\n%s", body)
	}
	if !strings.Contains(body, "#     grpc-transcode:") {
		t.Fatalf("expected gRPC plugin template in APISIX config, got:\n%s", body)
	}
	if !strings.Contains(body, "#     kafka-proxy:") {
		t.Fatalf("expected kafka plugin template in APISIX config, got:\n%s", body)
	}
}
