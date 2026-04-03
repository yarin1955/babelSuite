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

func TestRenderStandaloneConfigPromotesGRPCAndKafkaToActiveConfig(t *testing.T) {
	body := RenderStandaloneConfig(SuiteConfig{
		ID: "returns-control-plane",
		APISurfaces: []SurfaceConfig{
			{
				ID:       "pricing-service",
				Protocol: "gRPC",
				Operations: []OperationConfig{
					{
						ID:              "quote-refund",
						Method:          "RPC",
						Name:            "/returns.v1.RefundPricing/QuoteRefund",
						ContractPath:    "api/proto/refund_pricing.proto#QuoteRefund",
						ContractContent: "syntax = \"proto3\";\npackage returns_control_plane.v1;\nservice RefundPricing {\n  rpc QuoteRefund (QuoteRefundRequest) returns (QuoteRefundResponse);\n}\nmessage QuoteRefundRequest {}\nmessage QuoteRefundResponse {}\n",
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

	if !strings.Contains(body, "protos:") {
		t.Fatalf("expected embedded proto registry in APISIX config, got:\n%s", body)
	}
	if !strings.Contains(body, "id: returns-control-plane.quote-refund") {
		t.Fatalf("expected grpc proto id in APISIX config, got:\n%s", body)
	}
	if !strings.Contains(body, "name: grpc-transcode") {
		t.Fatalf("expected plugin catalog to include grpc-transcode, got:\n%s", body)
	}
	if !strings.Contains(body, "name: kafka-proxy") {
		t.Fatalf("expected plugin catalog to include kafka-proxy, got:\n%s", body)
	}
	if !strings.Contains(body, "grpc-transcode:") {
		t.Fatalf("expected active grpc-transcode route in APISIX config, got:\n%s", body)
	}
	if !strings.Contains(body, "scheme: grpc") {
		t.Fatalf("expected grpc upstream in APISIX config, got:\n%s", body)
	}
	if !strings.Contains(body, "resolver=/internal/mock-data/returns-control-plane/pricing-service/quote-refund") {
		t.Fatalf("expected gRPC resolver contract note, got:\n%s", body)
	}
	if !strings.Contains(body, "returns-control-plane.publish-refund-authorized (Kafka)") {
		t.Fatalf("expected Kafka resolver contract note, got:\n%s", body)
	}
	if !strings.Contains(body, "id: returns-control-plane.publish-refund-authorized.kafka") {
		t.Fatalf("expected active kafka route in APISIX config, got:\n%s", body)
	}
	if !strings.Contains(body, "scheme: kafka") {
		t.Fatalf("expected kafka upstream in APISIX config, got:\n%s", body)
	}
	if strings.Contains(body, "# Transports below still need APISIX-side plugin-runner") {
		t.Fatalf("did not expect deferred transport note for active grpc and kafka config, got:\n%s", body)
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
	if !strings.Contains(body, "stream_routes:") {
		t.Fatalf("expected active stream routes in APISIX config, got:\n%s", body)
	}
	if !strings.Contains(body, "upstreams:") {
		t.Fatalf("expected shared upstream objects in APISIX config, got:\n%s", body)
	}
	if !strings.Contains(body, "name: mqtt-proxy") {
		t.Fatalf("expected mqtt-proxy plugin in catalog, got:\n%s", body)
	}
	if !strings.Contains(body, "id: edge-lab.publish-order-created.kafka") {
		t.Fatalf("expected active Kafka route in APISIX config, got:\n%s", body)
	}
	if !strings.Contains(body, "id: edge-lab.publish-device-heartbeat.mqtt") {
		t.Fatalf("expected active MQTT stream route in APISIX config, got:\n%s", body)
	}
	if !strings.Contains(body, "mqtt-proxy:") {
		t.Fatalf("expected active MQTT plugin in APISIX config, got:\n%s", body)
	}
	if !strings.Contains(body, "id: edge-lab.accept-session.tcp") {
		t.Fatalf("expected active TCP stream route in APISIX config, got:\n%s", body)
	}
	if !strings.Contains(body, "server_port: 9200") {
		t.Fatalf("expected TCP stream listener in APISIX config, got:\n%s", body)
	}
	if !strings.Contains(body, "Stream-style transports such as MQTT/TCP/UDP require APISIX stream listeners") {
		t.Fatalf("expected stream transport guidance note, got:\n%s", body)
	}
	if strings.Contains(body, "# Transports below still need APISIX-side plugin-runner") {
		t.Fatalf("did not expect deferred transport note for active kafka, mqtt, and tcp config, got:\n%s", body)
	}
}

func TestRenderStandaloneConfigLeavesUnsupportedAsyncAMQPAndNATSDeferred(t *testing.T) {
	body := RenderStandaloneConfig(SuiteConfig{
		ID: "event-hub",
		APISurfaces: []SurfaceConfig{
			{
				ID:       "generic-events",
				Protocol: "Async",
				MockHost: "https://event-hub.mock.internal",
				Operations: []OperationConfig{
					{
						ID:      "publish-domain-event",
						Method:  "EVENT",
						Name:    "/events/domain",
						MockURL: "https://event-hub.mock.internal/events/domain",
						MockMetadata: OperationMetadataConfig{
							Adapter:     "async",
							ResolverURL: "/internal/mock-data/event-hub/generic-events/publish-domain-event",
							RuntimeURL:  "/mocks/async/event-hub/generic-events/publish-domain-event",
						},
					},
				},
			},
			{
				ID:       "billing-bus",
				Protocol: "AMQP",
				MockHost: "amqp://event-hub.mock.internal",
				Operations: []OperationConfig{
					{
						ID:      "publish-invoice-issued",
						Method:  "EVENT",
						Name:    "/billing.invoice.issued",
						MockURL: "amqp://event-hub.mock.internal/billing.invoice.issued",
						MockMetadata: OperationMetadataConfig{
							Adapter:     "async",
							ResolverURL: "/internal/mock-data/event-hub/billing-bus/publish-invoice-issued",
							RuntimeURL:  "/mocks/async/event-hub/billing-bus/publish-invoice-issued",
						},
					},
				},
			},
			{
				ID:       "ops-bus",
				Protocol: "NATS",
				MockHost: "nats://event-hub.mock.internal",
				Operations: []OperationConfig{
					{
						ID:      "publish-cache-invalidated",
						Method:  "EVENT",
						Name:    "/cache.invalidated",
						MockURL: "nats://event-hub.mock.internal/cache.invalidated",
						MockMetadata: OperationMetadataConfig{
							Adapter:     "async",
							ResolverURL: "/internal/mock-data/event-hub/ops-bus/publish-cache-invalidated",
							RuntimeURL:  "/mocks/async/event-hub/ops-bus/publish-cache-invalidated",
						},
					},
				},
			},
		},
	})

	if !strings.Contains(body, "# - event-hub.publish-domain-event (Async)") {
		t.Fatalf("expected deferred async note in APISIX config, got:\n%s", body)
	}
	if !strings.Contains(body, "# - event-hub.publish-invoice-issued (AMQP)") {
		t.Fatalf("expected deferred AMQP note in APISIX config, got:\n%s", body)
	}
	if !strings.Contains(body, "# - event-hub.publish-cache-invalidated (NATS)") {
		t.Fatalf("expected deferred NATS note in APISIX config, got:\n%s", body)
	}
	if strings.Contains(body, ".async") || strings.Contains(body, ".amqp") || strings.Contains(body, ".nats") {
		t.Fatalf("did not expect active unsupported routes in APISIX config, got:\n%s", body)
	}
	if !strings.Contains(body, "# Transports below still need APISIX-side plugin-runner") {
		t.Fatalf("expected unsupported transport note in APISIX config, got:\n%s", body)
	}
}
