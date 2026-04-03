package apisix

import (
	"strings"
	"testing"
)

func TestTransportKindInfersConcreteBrokerTransportFromAsyncScheme(t *testing.T) {
	testCases := []struct {
		name      string
		surface   SurfaceConfig
		operation OperationConfig
		expected  string
	}{
		{
			name: "async kafka host",
			surface: SurfaceConfig{
				Protocol: "Async",
				MockHost: "kafka://edge-lab.mock.internal",
			},
			operation: OperationConfig{
				MockURL: "kafka://edge-lab.mock.internal/orders.created",
				MockMetadata: OperationMetadataConfig{
					Adapter: "async",
				},
			},
			expected: "kafka",
		},
		{
			name: "async mqtt host",
			surface: SurfaceConfig{
				Protocol: "Async",
				MockHost: "mqtts://edge-lab.mock.internal",
			},
			operation: OperationConfig{
				MockURL: "mqtts://edge-lab.mock.internal/devices/heartbeat",
				MockMetadata: OperationMetadataConfig{
					Adapter: "async",
				},
			},
			expected: "mqtt",
		},
		{
			name: "async without broker scheme",
			surface: SurfaceConfig{
				Protocol: "Async",
				MockHost: "https://edge-lab.mock.internal",
			},
			operation: OperationConfig{
				MockURL: "/events/orders.created",
				MockMetadata: OperationMetadataConfig{
					Adapter: "async",
				},
			},
			expected: "async",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := transportKind(tc.surface, tc.operation)
			if actual != tc.expected {
				t.Fatalf("expected transport %q, got %q", tc.expected, actual)
			}
		})
	}
}

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

func TestRenderStandaloneConfigSupportsExtendedProtocols(t *testing.T) {
	body := RenderStandaloneConfig(SuiteConfig{
		ID: "edge-lab",
		APISurfaces: []SurfaceConfig{
			{
				ID:       "graph-api",
				Protocol: "GraphQL",
				MockHost: "https://edge-lab.mock.internal",
				Operations: []OperationConfig{
					{
						ID:      "catalog-query",
						Method:  "POST",
						Name:    "/graphql",
						MockURL: "https://edge-lab.mock.internal/graphql",
						MockMetadata: OperationMetadataConfig{
							Adapter:     "rest",
							ResolverURL: "/internal/mock-data/edge-lab/graph-api/catalog-query",
							RuntimeURL:  "/mocks/rest/edge-lab/graph-api/graphql",
						},
					},
				},
			},
			{
				ID:       "live-updates",
				Protocol: "WebSocket",
				MockHost: "wss://edge-lab.mock.internal",
				Operations: []OperationConfig{
					{
						ID:      "connect-feed",
						Method:  "GET",
						Name:    "/ws/feed",
						MockURL: "wss://edge-lab.mock.internal/ws/feed",
						MockMetadata: OperationMetadataConfig{
							Adapter:     "rest",
							ResolverURL: "/internal/mock-data/edge-lab/live-updates/connect-feed",
							RuntimeURL:  "/mocks/rest/edge-lab/live-updates/ws/feed",
						},
					},
				},
			},
			{
				ID:       "broker-feed",
				Protocol: "Kafka",
				MockHost: "kafka://edge-lab.mock.internal",
				Operations: []OperationConfig{
					{
						ID:      "publish-order-created",
						Method:  "EVENT",
						Name:    "/orders.created",
						MockURL: "kafka://edge-lab.mock.internal/orders.created",
						MockMetadata: OperationMetadataConfig{
							Adapter:     "async",
							ResolverURL: "/internal/mock-data/edge-lab/broker-feed/publish-order-created",
							RuntimeURL:  "/mocks/async/edge-lab/broker-feed/publish-order-created",
						},
					},
				},
			},
			{
				ID:       "device-bus",
				Protocol: "MQTT",
				MockHost: "mqtt://edge-lab.mock.internal",
				Operations: []OperationConfig{
					{
						ID:      "publish-device-heartbeat",
						Method:  "EVENT",
						Name:    "/devices/heartbeat",
						MockURL: "mqtt://edge-lab.mock.internal/devices/heartbeat",
						MockMetadata: OperationMetadataConfig{
							Adapter:     "async",
							ResolverURL: "/internal/mock-data/edge-lab/device-bus/publish-device-heartbeat",
							RuntimeURL:  "/mocks/async/edge-lab/device-bus/publish-device-heartbeat",
						},
					},
				},
			},
			{
				ID:       "raw-stream",
				Protocol: "TCP",
				MockHost: "tcp://edge-lab.mock.internal",
				Operations: []OperationConfig{
					{
						ID:      "accept-session",
						Method:  "EVENT",
						Name:    "/socket",
						MockURL: "tcp://edge-lab.mock.internal/socket",
						MockMetadata: OperationMetadataConfig{
							Adapter:     "async",
							ResolverURL: "/internal/mock-data/edge-lab/raw-stream/accept-session",
							RuntimeURL:  "/mocks/async/edge-lab/raw-stream/accept-session",
						},
					},
				},
			},
		},
	})

	if !strings.Contains(body, "uri: /graphql") {
		t.Fatalf("expected GraphQL route in APISIX config, got:\n%s", body)
	}
	if !strings.Contains(body, "name: degraphql") {
		t.Fatalf("expected degraphql plugin in catalog, got:\n%s", body)
	}
	if !strings.Contains(body, "enable_websocket: true") {
		t.Fatalf("expected websocket route support in APISIX config, got:\n%s", body)
	}
	if !strings.Contains(body, "name: mqtt-proxy") {
		t.Fatalf("expected mqtt-proxy plugin in catalog, got:\n%s", body)
	}
	if !strings.Contains(body, "edge-lab.publish-order-created (Kafka)") {
		t.Fatalf("expected deferred Kafka adapter note, got:\n%s", body)
	}
	if !strings.Contains(body, "edge-lab.publish-device-heartbeat (MQTT)") {
		t.Fatalf("expected deferred MQTT adapter note, got:\n%s", body)
	}
	if !strings.Contains(body, "#     mqtt-proxy:") {
		t.Fatalf("expected MQTT plugin template in APISIX config, got:\n%s", body)
	}
	if !strings.Contains(body, "#   server_port: 9200") {
		t.Fatalf("expected TCP stream-route template in APISIX config, got:\n%s", body)
	}
	if !strings.Contains(body, "Stream-style transports such as MQTT/TCP/UDP require APISIX stream listeners") {
		t.Fatalf("expected stream transport guidance note, got:\n%s", body)
	}
}
