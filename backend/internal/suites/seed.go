package suites

func seedSuites() map[string]Definition {
	return map[string]Definition{
		"payment-suite": {
			ID:          "payment-suite",
			Title:       "Payment Suite",
			Repository:  "localhost:5000/core-platform/payment-suite",
			Owner:       "Core Platform",
			Provider:    "Zot",
			Version:     "v2.4.1",
			Tags:        []string{"latest", "v2.4.1", "v2.4.0", "v2.3.8"},
			Description: "Bank-grade reference environment with Postgres, Kafka, Wiremock, and a full fraud worker topology.",
			Modules:     []string{"postgres", "kafka", "wiremock"},
			Status:      "Official",
			Score:       94,
			PullCommand: "babelctl run localhost:5000/core-platform/payment-suite:v2.4.1",
			ForkCommand: "babelctl fork localhost:5000/core-platform/payment-suite:v2.4.1 ./payment-suite-local",
			SuiteStar: `load("@babelsuite/postgres", "pg")
load("@babelsuite/kafka", "kafka")
load("@babelsuite/runtime", "container", "mock", "script", "scenario")

# Pre-registered Starlark Modules return strict structs.
db = container(name="db")
kafka = container(name="kafka")
stripe_mock = mock(name="stripe-mock", after=["db"])
bootstrap_topics = script(name="bootstrap-topics", after=["kafka"])
migrations = script(name="migrations", after=["db"])
payment_gateway = container(name="payment-gateway", after=["db", "stripe-mock", "migrations"])
fraud_worker = container(name="fraud-worker", after=["kafka", "bootstrap-topics", "payment-gateway"])
checkout_smoke = scenario(name="checkout-smoke", after=["payment-gateway", "fraud-worker"])`,
			Profiles: []ProfileOption{
				{FileName: "local.yaml", Label: "Local Debug", Description: "Verbose logs, local secrets, and relaxed timeouts.", Default: true},
				{FileName: "staging.yaml", Label: "Staging", Description: "Production-like service topology with Vault-backed references."},
				{FileName: "year.yaml", Label: "Year-End", Description: "Ledger rollover fixtures, end-of-year reporting toggles, and settlement edge cases."},
			},
			Folders: []FolderEntry{
				{Name: "profiles", Role: "Core", Description: "Environment variable toggles and runtime overrides.", Files: []string{"local.yaml", "staging.yaml", "year.yaml"}},
				{Name: "api", Role: "Core", Description: "Immutable OpenAPI and protobuf contracts for the suite.", Files: []string{"openapi/payments.yaml", "proto/payments.proto"}},
				{Name: "mock", Role: "Core", Description: "Wiremock mappings and scenario-specific stub bodies.", Files: []string{"wiremock/stripe/create-charge.json", "wiremock/stripe/refund.json"}},
				{Name: "scripts", Role: "Core", Description: "Boot-time migrations and broker preparation scripts.", Files: []string{"migrate.py", "bootstrap_topics.sh"}},
				{Name: "scenarios", Role: "Core", Description: "Smoke tests and attack-path executions.", Files: []string{"checkout_smoke.py", "refund_regression.py"}},
				{Name: "fixtures", Role: "Core", Description: "Static input data for cards, merchants, and seeded accounts.", Files: []string{"cards.json", "merchants.csv"}},
				{Name: "policies", Role: "Core", Description: "Rego payload validation and ledger invariants.", Files: []string{"ledger.rego", "pci.rego"}},
			},
			Contracts: []string{
				`Use load("@babelsuite/postgres", "pg") to provision the database and read db.url from the returned struct.`,
				"Kafka topics are exposed through a strict address contract so scenario containers never hand-craft broker URLs.",
				"Mocks live under mock/ and are selected through dispatch criteria rather than ad-hoc conditionals inside suite.star.",
			},
			APISurfaces: []APISurface{
				{
					ID:          "payment-gateway",
					Title:       "Payment Gateway",
					Protocol:    "REST",
					MockHost:    "https://payment-suite.mock.internal",
					Description: "Public checkout APIs backed by Wiremock-friendly examples and deterministic fraud scores.",
					Operations: []APIOperation{
						{
							ID:           "create-payment",
							Method:       "POST",
							Name:         "/payments",
							Summary:      "Create a payment authorization and fan out to Stripe.",
							ContractPath: "api/openapi/payments.yaml#/paths/~1payments/post",
							MockPath:     "mock/wiremock/stripe/create-charge.json",
							MockURL:      "https://payment-suite.mock.internal/payments?status=approved",
							CurlCommand:  `curl -X POST https://payment-suite.mock.internal/payments?status=approved -H 'content-type: application/json' -d '{"amount":1299,"currency":"USD","merchantId":"m-117"}'`,
							MockMetadata: MockOperationMetadata{
								Adapter:     "rest",
								DelayMillis: 80,
								ParameterConstraints: []ParameterConstraint{
									{Name: "status", Source: "query", Required: true, Pattern: "^(approved|review)$"},
									{Name: "content-type", Source: "header", Required: true, Pattern: "(?i)^application/json"},
									{Name: "x-suite-profile", Source: "header", Forward: true},
								},
								Fallback: &MockFallback{
									Mode:      "static",
									Status:    "422",
									MediaType: "application/json",
									Body:      "{\n  \"code\": \"UNKNOWN_PAYMENT_SCENARIO\",\n  \"message\": \"No payment mock matched the incoming scenario.\"\n}",
									Headers: []Header{
										{Name: "x-mock-fallback", Value: "payment-create"},
									},
								},
								State: &MockState{
									MutationKeyTemplate: "payment:{{ response.body.paymentId }}",
									Defaults: map[string]string{
										"status":      "created",
										"ledgerState": "pending",
									},
									Transitions: []MockStateTransition{
										{
											OnExample: "approved-card",
											Set: map[string]string{
												"status":      "authorized",
												"ledgerState": "posted",
												"profile":     "{{ request.headers.x-suite-profile }}",
											},
										},
										{
											OnExample: "fraud-review",
											Set: map[string]string{
												"status":      "pending_review",
												"ledgerState": "held",
												"profile":     "{{ request.headers.x-suite-profile }}",
												"fraudScore":  "{{ request.headers.x-fraud-score }}",
											},
										},
									},
								},
							},
							Exchanges: []ExchangeExample{
								{
									Name:           "approved-card",
									SourceArtifact: "wiremock/stripe/create-charge.json",
									When:           []MatchCondition{{From: "query", Param: "status", Value: "approved"}},
									RequestHeaders: []Header{
										{Name: "content-type", Value: "application/json"},
										{Name: "x-suite-profile", Value: "local.yaml"},
									},
									RequestBody: `{
  "amount": 1299,
  "currency": "USD",
  "merchantId": "m-117",
  "cardToken": "tok_visa"
}`,
									ResponseStatus:    "201",
									ResponseMediaType: "application/json",
									ResponseHeaders: []Header{
										{Name: "x-mock-source", Value: "stripe-approved"},
										{Name: "cache-control", Value: "no-store"},
									},
									ResponseBody: `{
  "paymentId": "pay_1043",
  "status": "authorized",
  "processor": "stripe-mock"
}`,
								},
								{
									Name:           "fraud-review",
									SourceArtifact: "wiremock/stripe/create-charge.json",
									When:           []MatchCondition{{From: "query", Param: "status", Value: "review"}},
									RequestHeaders: []Header{
										{Name: "content-type", Value: "application/json"},
										{Name: "x-fraud-score", Value: "87"},
									},
									RequestBody: `{
  "amount": 9900,
  "currency": "USD",
  "merchantId": "m-441",
  "cardToken": "tok_risky"
}`,
									ResponseStatus:    "202",
									ResponseMediaType: "application/json",
									ResponseHeaders: []Header{
										{Name: "x-review-queue", Value: "fraud-worker"},
									},
									ResponseBody: `{
  "paymentId": "pay_2048",
  "status": "pending_review",
  "reason": "fraud-score-threshold"
}`,
								},
							},
						},
						{
							ID:           "get-payment",
							Method:       "GET",
							Name:         "/payments/{paymentId}",
							Summary:      "Retrieve a previously authorized payment and ledger status.",
							ContractPath: "api/openapi/payments.yaml#/paths/~1payments~1{paymentId}/get",
							MockPath:     "mock/wiremock/stripe/get-payment.json",
							MockURL:      "https://payment-suite.mock.internal/payments/pay_1043",
							CurlCommand:  `curl https://payment-suite.mock.internal/payments/pay_1043 -H 'accept: application/json'`,
							MockMetadata: MockOperationMetadata{
								Adapter: "rest",
								ParameterConstraints: []ParameterConstraint{
									{Name: "paymentId", Source: "path", Required: true, Pattern: "^pay_[A-Za-z0-9]+$"},
									{Name: "x-request-id", Source: "header", Forward: true},
								},
								Fallback: &MockFallback{
									Mode:      "static",
									Status:    "404",
									MediaType: "application/json",
									Body:      "{\n  \"code\": \"PAYMENT_NOT_FOUND\",\n  \"message\": \"No payment state exists for {{ request.path.paymentId }}.\"\n}",
								},
								State: &MockState{
									LookupKeyTemplate: "payment:{{ request.path.paymentId }}",
									Defaults: map[string]string{
										"status":      "authorized",
										"ledgerState": "posted",
										"profile":     "local.yaml",
									},
								},
							},
							Exchanges: []ExchangeExample{
								{
									Name:           "authorized",
									SourceArtifact: "wiremock/stripe/get-payment.json",
									When:           []MatchCondition{{From: "path", Param: "paymentId", Value: "pay_1043"}},
									RequestHeaders: []Header{
										{Name: "accept", Value: "application/json"},
									},
									RequestBody:       "",
									ResponseStatus:    "200",
									ResponseMediaType: "application/json",
									ResponseHeaders: []Header{
										{Name: "etag", Value: `W/"pay_1043"`},
									},
									ResponseBody: `{
  "paymentId": "{{ request.path.paymentId }}",
  "status": "{{ state.status }}",
  "ledgerState": "{{ state.ledgerState }}",
  "profile": "{{ state.profile }}"
}`,
								},
							},
						},
					},
				},
			},
		},
		"fleet-control-room": {
			ID:          "fleet-control-room",
			Title:       "Fleet Control Room",
			Repository:  "localhost:5000/platform/fleet-control-room",
			Owner:       "Mobility QA",
			Provider:    "Zot",
			Version:     "v1.8.0",
			Tags:        []string{"latest", "v1.8.0", "v1.7.5"},
			Description: "End-to-end vehicle orchestration environment with Redis, gRPC contracts, and mocked telemetry spikes.",
			Modules:     []string{"redis", "grpc", "prometheus"},
			Status:      "Verified",
			Score:       88,
			PullCommand: "babelctl run localhost:5000/platform/fleet-control-room:v1.8.0",
			ForkCommand: "babelctl fork localhost:5000/platform/fleet-control-room:v1.8.0 ./fleet-lab",
			SuiteStar: `load("@babelsuite/redis", "redis")
load("@babelsuite/runtime", "container", "mock", "script", "scenario")

redis_cache = container(name="redis-cache")
telemetry_mock = mock(name="telemetry-mock", after=["redis-cache"])
seed_routes = script(name="seed-routes", after=["redis-cache"])
dispatcher_api = container(name="dispatcher-api", after=["redis-cache", "seed-routes"])
planner = container(name="route-planner", after=["dispatcher-api"])
control_room = container(name="control-room-ui", after=["dispatcher-api", "route-planner"])
fleet_smoke = scenario(name="fleet-smoke", after=["control-room-ui", "telemetry-mock"])`,
			Profiles: []ProfileOption{
				{FileName: "local.yaml", Label: "Local Debug", Description: "Browser-forwarded ports and verbose telemetry payloads.", Default: true},
				{FileName: "perf.yaml", Label: "Performance", Description: "Synthetic bursts for planner saturation tests."},
				{FileName: "staging.yaml", Label: "Staging", Description: "Shared staging identities and realistic routing backends."},
			},
			Folders: []FolderEntry{
				{Name: "profiles", Role: "Core", Description: "Driver-specific runtime knobs for local, perf, and staging lanes.", Files: []string{"local.yaml", "perf.yaml", "staging.yaml"}},
				{Name: "api", Role: "Core", Description: "gRPC protobuf definitions and REST gateway overlays.", Files: []string{"proto/fleet_control.proto", "openapi/dispatcher.yaml"}},
				{Name: "mock", Role: "Core", Description: "Telemetry playback feeds and fault injections for route spikes.", Files: []string{"telemetry/spike.json", "telemetry/idle.json"}},
				{Name: "scripts", Role: "Core", Description: "Redis seeders and topology bootstrap hooks.", Files: []string{"seed_routes.sh", "prime_cache.py"}},
				{Name: "scenarios", Role: "Core", Description: "Control room smoke runs and degraded GPS scenarios.", Files: []string{"fleet_smoke.py", "route_degradation.py"}},
				{Name: "fixtures", Role: "Core", Description: "Vehicle manifests and fake GPS frames.", Files: []string{"vehicles.json", "gps_frames.ndjson"}},
				{Name: "policies", Role: "Core", Description: "Route SLA validation and forbidden-zone checks.", Files: []string{"route_latency.rego", "geo_boundary.rego"}},
			},
			Contracts: []string{
				"The dispatcher API reads strict outputs from redis-cache instead of inferring connection details from container names.",
				"Telemetry mocks are treated as first-class topology nodes so they can be selected, filtered, and replayed from the UI.",
				"gRPC contracts in api/ remain immutable while mock payloads in mock/ evolve alongside scenario coverage.",
			},
			APISurfaces: []APISurface{
				{
					ID:          "dispatcher-api",
					Title:       "Dispatcher API",
					Protocol:    "gRPC",
					MockHost:    "grpc://fleet-control-room.mock.internal",
					Description: "Planner control APIs exposed through protobuf contracts with a mocked telemetry side-channel.",
					Operations: []APIOperation{
						{
							ID:           "assign-route",
							Method:       "RPC",
							Name:         "fleet.v1.Dispatcher/AssignRoute",
							Summary:      "Assign a route to a vehicle and publish routing metadata.",
							ContractPath: "api/proto/fleet_control.proto#AssignRoute",
							MockPath:     "mock/telemetry/spike.json",
							MockURL:      "grpc://fleet-control-room.mock.internal/fleet.v1.Dispatcher/AssignRoute",
							CurlCommand:  `grpcurl -plaintext -d '{"vehicleId":"vh-11","routeId":"route-778"}' fleet-control-room.mock.internal fleet.v1.Dispatcher/AssignRoute`,
							MockMetadata: MockOperationMetadata{
								Adapter: "grpc",
								ParameterConstraints: []ParameterConstraint{
									{Name: "x-profile", Source: "header", Forward: true},
								},
								Fallback: &MockFallback{
									Mode:      "static",
									Status:    "5",
									MediaType: "application/json",
									Body:      "{\n  \"code\": \"ROUTE_ASSIGNMENT_NOT_FOUND\",\n  \"message\": \"No gRPC exchange matched the incoming vehicle.\" \n}",
								},
							},
							Exchanges: []ExchangeExample{
								{
									Name:           "urban-shift",
									SourceArtifact: "mock/telemetry/spike.json",
									When:           []MatchCondition{{From: "body", Param: "vehicleId", Value: "vh-11"}},
									RequestHeaders: []Header{
										{Name: "x-profile", Value: "perf.yaml"},
									},
									RequestBody: `{
  "vehicleId": "vh-11",
  "routeId": "route-778"
}`,
									ResponseStatus:    "0",
									ResponseMediaType: "application/grpc",
									ResponseHeaders: []Header{
										{Name: "x-topology-wave", Value: "planner"},
									},
									ResponseBody: `{
  "assignmentId": "asg-778",
  "status": "accepted",
  "plannerRevision": "route-planner@4"
}`,
								},
							},
						},
						{
							ID:           "stream-telemetry",
							Method:       "POST",
							Name:         "/telemetry/events",
							Summary:      "Inject mock telemetry frames into the control room pipeline.",
							ContractPath: "api/openapi/dispatcher.yaml#/paths/~1telemetry~1events/post",
							MockPath:     "mock/telemetry/idle.json",
							MockURL:      "https://fleet-control-room.mock.internal/telemetry/events?scenario=idle",
							CurlCommand:  `curl -X POST https://fleet-control-room.mock.internal/telemetry/events?scenario=idle -H 'content-type: application/json' -d '{"vehicleId":"vh-11","speed":0}'`,
							Exchanges: []ExchangeExample{
								{
									Name:           "idle-garage",
									SourceArtifact: "mock/telemetry/idle.json",
									When:           []MatchCondition{{From: "query", Param: "scenario", Value: "idle"}},
									RequestHeaders: []Header{
										{Name: "content-type", Value: "application/json"},
									},
									RequestBody: `{
  "vehicleId": "vh-11",
  "speed": 0,
  "battery": 76
}`,
									ResponseStatus:    "202",
									ResponseMediaType: "application/json",
									ResponseHeaders: []Header{
										{Name: "x-ingest-batch", Value: "telemetry-mock"},
									},
									ResponseBody: `{
  "accepted": true,
  "scenario": "idle",
  "framesQueued": 1
}`,
								},
							},
						},
					},
				},
			},
		},
		"storefront-browser-lab": {
			ID:          "storefront-browser-lab",
			Title:       "Storefront Browser Lab",
			Repository:  "localhost:5000/qa/storefront-browser-lab",
			Owner:       "Release Engineering",
			Provider:    "Zot",
			Version:     "v1.3.0",
			Tags:        []string{"latest", "v1.3.0", "v1.2.5"},
			Description: "Browser-first commerce lab with Kafka event streams, Playwright checkout journeys, and mock APIs for promo traffic validation.",
			Modules:     []string{"kafka", "playwright", "mock-api"},
			Status:      "Verified",
			Score:       90,
			PullCommand: "babelctl run localhost:5000/qa/storefront-browser-lab:v1.3.0",
			ForkCommand: "babelctl fork localhost:5000/qa/storefront-browser-lab:v1.3.0 ./storefront-browser-lab",
			SuiteStar: `load("@babelsuite/kafka", "kafka")
load("@babelsuite/playwright", "playwright")
load("@babelsuite/runtime", "container", "mock", "script", "scenario")

broker = container(name="kafka")
catalog_mock = mock(name="catalog-mock", after=["kafka"])
orders_mock = mock(name="orders-mock", after=["kafka"])
seed_topics = script(name="seed-topics", after=["kafka"])
event_consumer = container(name="event-consumer", after=["kafka", "seed-topics", "orders-mock"])
storefront_api = container(name="storefront-api", after=["catalog-mock", "orders-mock", "seed-topics"])
storefront_ui = container(name="storefront-ui", after=["storefront-api"])
playwright_checkout = scenario(name="playwright-checkout", after=["storefront-ui", "event-consumer"])`,
			Profiles: []ProfileOption{
				{FileName: "local.yaml", Label: "Local Debug", Description: "Opens browser traces, seeded demo users, and single-worker Kafka consumption.", Default: true},
				{FileName: "ci.yaml", Label: "CI Browser", Description: "Headless Playwright suite with tighter timeouts and deterministic mocks."},
				{FileName: "promo.yaml", Label: "Promo Burst", Description: "High-throughput cart and checkout traffic with promotional catalog fixtures."},
			},
			Folders: []FolderEntry{
				{Name: "profiles", Role: "Core", Description: "Browser, Kafka, and mock dispatch settings for local, CI, and campaign traffic.", Files: []string{"local.yaml", "ci.yaml", "promo.yaml"}},
				{Name: "api", Role: "Core", Description: "Order and catalog contracts exposed to the UI and background consumer.", Files: []string{"openapi/orders.yaml", "proto/storefront_events.proto"}},
				{Name: "mock", Role: "Core", Description: "Mock API payloads for product catalog and order submission paths.", Files: []string{"catalog/list-products.json", "orders/create-order.json"}},
				{Name: "scripts", Role: "Core", Description: "Kafka bootstrap and browser fixture warm-up hooks.", Files: []string{"seed_topics.sh", "warm_cache.ts"}},
				{Name: "scenarios", Role: "Core", Description: "Playwright coverage for checkout success and cart abandonment journeys.", Files: []string{"playwright_checkout.spec.ts", "cart_abandonment.spec.ts"}},
				{Name: "fixtures", Role: "Core", Description: "Seeded products, campaigns, and browser-side user sessions.", Files: []string{"products.json", "users.json"}},
				{Name: "policies", Role: "Core", Description: "Event schema and checkout latency validation rules.", Files: []string{"event_schema.rego", "checkout_latency.rego"}},
			},
			Contracts: []string{
				"Playwright scenarios consume strict base URLs from suite outputs rather than hard-coded localhost ports.",
				"Kafka publishes checkout and cart events through a fixed topic contract so browser assertions can wait on stable signals.",
				"Mock APIs under mock/ provide deterministic catalog and order responses without changing the immutable api/ contracts.",
			},
			APISurfaces: []APISurface{
				{
					ID:          "storefront-api",
					Title:       "Storefront Orders API",
					Protocol:    "REST",
					MockHost:    "https://storefront-browser-lab.mock.internal",
					Description: "Catalog and order endpoints served through deterministic mock APIs so Playwright can validate browser flows without backend drift.",
					Operations: []APIOperation{
						{
							ID:           "list-products",
							Method:       "GET",
							Name:         "/catalog/products",
							Summary:      "Return the product grid shown to the storefront browser before checkout begins.",
							ContractPath: "api/openapi/orders.yaml#/paths/~1catalog~1products/get",
							MockPath:     "mock/catalog/list-products.json",
							MockURL:      "https://storefront-browser-lab.mock.internal/catalog/products?scenario=promo",
							CurlCommand:  `curl "https://storefront-browser-lab.mock.internal/catalog/products?scenario=promo" -H "accept: application/json"`,
							MockMetadata: MockOperationMetadata{
								Adapter: "rest",
								ParameterConstraints: []ParameterConstraint{
									{Name: "scenario", Source: "query", Required: true, Pattern: "^(promo)$"},
									{Name: "accept", Source: "header", Required: true, Pattern: "(?i)application/json"},
								},
								Fallback: &MockFallback{
									Mode:      "static",
									Status:    "200",
									MediaType: "application/json",
									Body:      "{\n  \"products\": [],\n  \"campaign\": \"default\"\n}",
									Headers: []Header{
										{Name: "x-mock-fallback", Value: "catalog-empty"},
									},
								},
							},
							Exchanges: []ExchangeExample{
								{
									Name:           "promo-grid",
									SourceArtifact: "mock/catalog/list-products.json",
									When:           []MatchCondition{{From: "query", Param: "scenario", Value: "promo"}},
									RequestHeaders: []Header{
										{Name: "accept", Value: "application/json"},
										{Name: "x-suite-profile", Value: "promo.yaml"},
									},
									RequestBody:       "",
									ResponseStatus:    "200",
									ResponseMediaType: "application/json",
									ResponseHeaders: []Header{
										{Name: "x-mock-source", Value: "catalog-promo"},
									},
									ResponseBody: `{
  "products": [
    { "sku": "sku_1001", "name": "Starter Keyboard", "price": 4900 },
    { "sku": "sku_2024", "name": "Launch Headset", "price": 12900 }
  ],
  "campaign": "spring-promo"
}`,
								},
							},
						},
						{
							ID:           "create-order",
							Method:       "POST",
							Name:         "/orders",
							Summary:      "Submit a checkout order and emit a Kafka event consumed by the browser verification lane.",
							ContractPath: "api/openapi/orders.yaml#/paths/~1orders/post",
							MockPath:     "mock/orders/create-order.json",
							MockURL:      "https://storefront-browser-lab.mock.internal/orders?scenario=happy-path",
							CurlCommand:  `curl -X POST "https://storefront-browser-lab.mock.internal/orders?scenario=happy-path" -H "content-type: application/json" -d '{"sku":"sku_1001","quantity":1,"email":"shopper@demo.test"}'`,
							MockMetadata: MockOperationMetadata{
								Adapter: "rest",
								ParameterConstraints: []ParameterConstraint{
									{Name: "scenario", Source: "query", Required: true, Pattern: "^(happy-path|out-of-stock)$"},
									{Name: "content-type", Source: "header", Required: true, Pattern: "(?i)^application/json"},
									{Name: "x-browser-suite", Source: "header", Forward: true},
								},
								Fallback: &MockFallback{
									Mode:      "static",
									Status:    "422",
									MediaType: "application/json",
									Body:      "{\n  \"code\": \"UNKNOWN_ORDER_SCENARIO\",\n  \"message\": \"No order mock matched the incoming scenario.\"\n}",
								},
								State: &MockState{
									LookupKeyTemplate:   "checkout-attempt:{{ request.body.email }}",
									MutationKeyTemplate: "checkout-attempt:{{ request.body.email }}",
									Defaults: map[string]string{
										"count": "0",
									},
									Transitions: []MockStateTransition{
										{
											OnExample: "happy-path",
											Set: map[string]string{
												"lastOrderId": "{{ response.body.orderId }}",
												"lastStatus":  "accepted",
											},
											Increment: map[string]int{
												"count": 1,
											},
										},
										{
											OnExample: "out-of-stock",
											Set: map[string]string{
												"lastStatus": "rejected",
												"reason":     "inventory",
											},
											Increment: map[string]int{
												"count": 1,
											},
										},
									},
								},
							},
							Exchanges: []ExchangeExample{
								{
									Name:           "happy-path",
									SourceArtifact: "mock/orders/create-order.json",
									When:           []MatchCondition{{From: "query", Param: "scenario", Value: "happy-path"}},
									RequestHeaders: []Header{
										{Name: "content-type", Value: "application/json"},
										{Name: "x-browser-suite", Value: "playwright"},
									},
									RequestBody: `{
  "sku": "sku_1001",
  "quantity": 1,
  "email": "shopper@demo.test"
}`,
									ResponseStatus:    "201",
									ResponseMediaType: "application/json",
									ResponseHeaders: []Header{
										{Name: "x-kafka-topic", Value: "checkout-events"},
									},
									ResponseBody: `{
  "orderId": "ord_7001",
  "status": "accepted",
  "eventPublished": true,
  "previousAttempts": "{{ state.count }}"
}`,
								},
								{
									Name:           "out-of-stock",
									SourceArtifact: "mock/orders/create-order.json",
									When:           []MatchCondition{{From: "query", Param: "scenario", Value: "out-of-stock"}},
									RequestHeaders: []Header{
										{Name: "content-type", Value: "application/json"},
									},
									RequestBody: `{
  "sku": "sku_4040",
  "quantity": 1,
  "email": "shopper@demo.test"
}`,
									ResponseStatus:    "409",
									ResponseMediaType: "application/json",
									ResponseHeaders: []Header{
										{Name: "x-mock-source", Value: "inventory-guard"},
									},
									ResponseBody: `{
  "code": "OUT_OF_STOCK",
  "message": "Requested item is not available.",
  "previousAttempts": "{{ state.count }}"
}`,
								},
							},
						},
					},
				},
			},
		},
		"returns-control-plane": {
			ID:          "returns-control-plane",
			Title:       "Returns Control Plane",
			Repository:  "localhost:5000/qa/returns-control-plane",
			Owner:       "Commerce QA",
			Provider:    "Zot",
			Version:     "v1.0.0",
			Tags:        []string{"latest", "v1.0.0", "v0.9.2"},
			Description: "Reverse-logistics suite that shows BabelSuite's richer mock runtime with split metadata, templating, constraints, fallback, state, and multi-protocol surfaces.",
			Modules:     []string{"postgres", "kafka", "grpc", "mock-api"},
			Status:      "Verified",
			Score:       92,
			PullCommand: "babelctl run localhost:5000/qa/returns-control-plane:v1.0.0",
			ForkCommand: "babelctl fork localhost:5000/qa/returns-control-plane:v1.0.0 ./returns-control-plane-local",
			SuiteStar: `load("@babelsuite/kafka", "kafka")
load("@babelsuite/postgres", "pg")
load("@babelsuite/runtime", "container", "mock", "script", "scenario")

returns_db = container(name="returns-db")
broker = container(name="kafka")
refunds_mock = mock(name="refunds-mock", after=["returns-db"])
pricing_mock = mock(name="pricing-mock", after=["returns-db"])
events_mock = mock(name="events-mock", after=["kafka"])
seed_topics = script(name="seed-topics", after=["kafka"])
seed_routes = script(name="seed-routes", after=["returns-db", "refunds-mock", "pricing-mock"])
returns_api = container(name="returns-api", after=["returns-db", "refunds-mock", "pricing-mock", "seed-routes"])
refund_worker = container(name="refund-worker", after=["kafka", "returns-api", "events-mock", "seed-topics"])
returns_smoke = scenario(name="returns-smoke", after=["returns-api", "refund-worker"])`,
			Profiles: []ProfileOption{
				{FileName: "local.yaml", Label: "Local Debug", Description: "Verbose logs, seeded return ids, and open worker traces.", Default: true},
				{FileName: "canary.yaml", Label: "Canary Routing", Description: "Canary refund rules with manual-review escalation and delayed callbacks."},
				{FileName: "peak.yaml", Label: "Peak Season", Description: "High-volume return traffic with fraud screening and worker backpressure."},
			},
			Folders: []FolderEntry{
				{Name: "profiles", Role: "Core", Description: "Launch profiles for local, canary, and peak-season refund traffic.", Files: []string{"local.yaml", "canary.yaml", "peak.yaml"}},
				{Name: "api", Role: "Core", Description: "OpenAPI and protobuf contracts for returns and refund pricing.", Files: []string{"openapi/returns.yaml", "proto/refund_pricing.proto"}},
				{Name: "mock", Role: "Core", Description: "Mock payloads plus metadata that control dispatch, fallback, and state.", Files: []string{"returns/create-return.json", "returns/get-return.json", "pricing/quote-refund.json", "events/refund-authorized.json"}},
				{Name: "scripts", Role: "Core", Description: "Bootstrap hooks for Kafka topics and refund routing tables.", Files: []string{"seed_topics.sh", "seed_routes.ts"}},
				{Name: "scenarios", Role: "Core", Description: "Smoke and manual-review scenarios for reverse logistics flows.", Files: []string{"returns_smoke.py", "manual_review.spec.ts"}},
				{Name: "fixtures", Role: "Core", Description: "Seeded return cases and customer profiles.", Files: []string{"customers.json", "returns.json"}},
				{Name: "policies", Role: "Core", Description: "Refund-limit and event-schema validation policies.", Files: []string{"refund_limits.rego", "event_schema.rego"}},
			},
			Contracts: []string{
				"Examples live in mock/ while runtime behavior is split into adjacent metadata files so dispatch logic can evolve without rewriting payloads.",
				"Return creation writes persisted mock state that the lookup operation renders later through templates.",
				"REST, gRPC, and async surfaces share one mock engine while keeping protocol-specific runtime adapters at the edge.",
			},
			APISurfaces: []APISurface{
				{
					ID:          "returns-api",
					Title:       "Returns API",
					Protocol:    "REST",
					MockHost:    "https://returns-control-plane.mock.internal",
					Description: "REST surface for creating and reading returns with stateful decisions and strict parameter validation.",
					Operations: []APIOperation{
						{
							ID:           "create-return",
							Method:       "POST",
							Name:         "/returns",
							Summary:      "Create a return request and decide whether it is auto-approved or escalated to manual review.",
							ContractPath: "api/openapi/returns.yaml#/paths/~1returns/post",
							MockPath:     "mock/returns/create-return.json",
							MockURL:      "https://returns-control-plane.mock.internal/returns?scenario=approved",
							CurlCommand:  `curl -X POST "https://returns-control-plane.mock.internal/returns?scenario=approved" -H "content-type: application/json" -H "x-suite-profile: canary.yaml" -d '{"orderId":"ord_1001","returnReason":"damaged","amountCents":4200}'`,
							MockMetadata: MockOperationMetadata{
								Adapter:     "rest",
								DelayMillis: 120,
								RuntimeURL:  "/mocks/rest/returns-control-plane/returns-api/returns?scenario=approved",
								ParameterConstraints: []ParameterConstraint{
									{Name: "scenario", Source: "query", Required: true, Pattern: "^(approved|manual-review)$"},
									{Name: "content-type", Source: "header", Required: true, Pattern: "(?i)^application/json"},
									{Name: "x-suite-profile", Source: "header", Forward: true},
								},
								Fallback: &MockFallback{
									Mode:      "static",
									Status:    "422",
									MediaType: "application/json",
									Body:      "{\n  \"code\": \"UNKNOWN_RETURN_SCENARIO\",\n  \"message\": \"No return mock matched the incoming scenario.\"\n}",
								},
								State: &MockState{
									MutationKeyTemplate: "return:{{ response.body.returnId }}",
									Defaults: map[string]string{
										"status":      "created",
										"refundMode":  "pending",
										"reviewQueue": "triage",
									},
									Transitions: []MockStateTransition{
										{
											OnExample: "approved-standard",
											Set: map[string]string{
												"status":      "approved",
												"refundMode":  "instant_credit",
												"reviewQueue": "auto",
												"profile":     "{{ request.headers.x-suite-profile }}",
											},
										},
										{
											OnExample: "manual-review",
											Set: map[string]string{
												"status":      "manual_review",
												"refundMode":  "agent_review",
												"reviewQueue": "manual",
												"profile":     "{{ request.headers.x-suite-profile }}",
											},
										},
									},
								},
							},
							Exchanges: []ExchangeExample{
								{
									Name:           "approved-standard",
									SourceArtifact: "mock/returns/create-return.json",
									When:           []MatchCondition{{From: "query", Param: "scenario", Value: "approved"}},
									RequestHeaders: []Header{
										{Name: "content-type", Value: "application/json"},
										{Name: "x-suite-profile", Value: "canary.yaml"},
									},
									RequestBody: `{
  "orderId": "ord_1001",
  "returnReason": "damaged",
  "amountCents": 4200
}`,
									ResponseStatus:    "201",
									ResponseMediaType: "application/json",
									ResponseHeaders: []Header{
										{Name: "x-mock-source", Value: "returns-approved"},
									},
									ResponseBody: `{
  "returnId": "ret_1001",
  "status": "approved",
  "refundMode": "instant_credit",
  "queue": "auto"
}`,
								},
								{
									Name:           "manual-review",
									SourceArtifact: "mock/returns/create-return.json",
									When:           []MatchCondition{{From: "query", Param: "scenario", Value: "manual-review"}},
									RequestHeaders: []Header{
										{Name: "content-type", Value: "application/json"},
										{Name: "x-suite-profile", Value: "peak.yaml"},
									},
									RequestBody: `{
  "orderId": "ord_2042",
  "returnReason": "suspicious_activity",
  "amountCents": 15900
}`,
									ResponseStatus:    "202",
									ResponseMediaType: "application/json",
									ResponseHeaders: []Header{
										{Name: "x-review-queue", Value: "manual"},
									},
									ResponseBody: `{
  "returnId": "ret_2002",
  "status": "manual_review",
  "refundMode": "agent_review",
  "queue": "manual"
}`,
								},
							},
						},
						{
							ID:           "get-return",
							Method:       "GET",
							Name:         "/returns/{returnId}",
							Summary:      "Fetch the current return state after creation has mutated mock runtime state.",
							ContractPath: "api/openapi/returns.yaml#/paths/~1returns~1{returnId}/get",
							MockPath:     "mock/returns/get-return.json",
							MockURL:      "https://returns-control-plane.mock.internal/returns/ret_1001",
							CurlCommand:  `curl "https://returns-control-plane.mock.internal/returns/ret_1001" -H "x-request-id: req-100"`,
							MockMetadata: MockOperationMetadata{
								Adapter:    "rest",
								RuntimeURL: "/mocks/rest/returns-control-plane/returns-api/returns/ret_1001",
								ParameterConstraints: []ParameterConstraint{
									{Name: "returnId", Source: "path", Required: true, Pattern: "^ret_[0-9]+$"},
									{Name: "x-request-id", Source: "header", Forward: true},
								},
								Fallback: &MockFallback{
									Mode:      "static",
									Status:    "404",
									MediaType: "application/json",
									Body:      "{\n  \"code\": \"RETURN_NOT_FOUND\",\n  \"message\": \"No persisted state exists for {{ request.path.returnId }}.\"\n}",
								},
								State: &MockState{
									LookupKeyTemplate: "return:{{ request.path.returnId }}",
									Defaults: map[string]string{
										"status":      "approved",
										"refundMode":  "instant_credit",
										"reviewQueue": "auto",
										"profile":     "local.yaml",
									},
								},
							},
							Exchanges: []ExchangeExample{
								{
									Name:           "return-state",
									SourceArtifact: "mock/returns/get-return.json",
									When:           []MatchCondition{{From: "path", Param: "returnId", Value: "ret_1001"}},
									RequestHeaders: []Header{
										{Name: "x-request-id", Value: "req-100"},
									},
									ResponseStatus:    "200",
									ResponseMediaType: "application/json",
									ResponseHeaders: []Header{
										{Name: "x-mock-source", Value: "returns-state"},
									},
									ResponseBody: `{
  "returnId": "{{ request.path.returnId }}",
  "status": "{{ state.status }}",
  "refundMode": "{{ state.refundMode }}",
  "reviewQueue": "{{ state.reviewQueue }}",
  "profile": "{{ state.profile }}"
}`,
								},
							},
						},
					},
				},
				{
					ID:          "pricing-service",
					Title:       "Refund Pricing",
					Protocol:    "gRPC",
					MockHost:    "grpc://returns-control-plane.mock.internal",
					Description: "gRPC refund-quote surface that reuses the shared mock engine through a thin protocol adapter.",
					Operations: []APIOperation{
						{
							ID:           "quote-refund",
							Method:       "RPC",
							Name:         "returns.v1.RefundPricing/QuoteRefund",
							Summary:      "Quote the refund amount and decide whether the request is low-risk or needs manual review.",
							ContractPath: "api/proto/refund_pricing.proto#QuoteRefund",
							MockPath:     "mock/pricing/quote-refund.json",
							MockURL:      "grpc://returns-control-plane.mock.internal/returns.v1.RefundPricing/QuoteRefund",
							CurlCommand:  `grpcurl -plaintext -d '{"returnReason":"damaged","amountCents":4200,"currency":"USD"}' returns-control-plane.mock.internal returns.v1.RefundPricing/QuoteRefund`,
							MockMetadata: MockOperationMetadata{
								Adapter:    "grpc",
								RuntimeURL: "/mocks/grpc/returns-control-plane/pricing-service/quote-refund",
								ParameterConstraints: []ParameterConstraint{
									{Name: "x-risk-tier", Source: "header", Forward: true},
								},
								Fallback: &MockFallback{
									Mode:      "static",
									Status:    "5",
									MediaType: "application/json",
									Body:      "{\n  \"code\": \"QUOTE_NOT_AVAILABLE\",\n  \"message\": \"No refund quote mock matched the request.\"\n}",
								},
							},
							Exchanges: []ExchangeExample{
								{
									Name:           "low-risk",
									SourceArtifact: "mock/pricing/quote-refund.json",
									When:           []MatchCondition{{From: "body", Param: "returnReason", Value: "damaged"}},
									RequestHeaders: []Header{
										{Name: "x-risk-tier", Value: "low"},
									},
									RequestBody: `{
  "returnReason": "damaged",
  "amountCents": 4200,
  "currency": "USD"
}`,
									ResponseStatus:    "0",
									ResponseMediaType: "application/json",
									ResponseHeaders: []Header{
										{Name: "x-pricing-path", Value: "instant"},
									},
									ResponseBody: `{
  "quoteId": "quote_301",
  "decision": "instant",
  "refundCents": 4200
}`,
								},
								{
									Name:           "fraud-screen",
									SourceArtifact: "mock/pricing/quote-refund.json",
									When:           []MatchCondition{{From: "body", Param: "returnReason", Value: "suspicious_activity"}},
									RequestHeaders: []Header{
										{Name: "x-risk-tier", Value: "high"},
									},
									RequestBody: `{
  "returnReason": "suspicious_activity",
  "amountCents": 15900,
  "currency": "USD"
}`,
									ResponseStatus:    "0",
									ResponseMediaType: "application/json",
									ResponseHeaders: []Header{
										{Name: "x-pricing-path", Value: "manual_review"},
									},
									ResponseBody: `{
  "quoteId": "quote_911",
  "decision": "manual_review",
  "refundCents": 0
}`,
								},
							},
						},
					},
				},
				{
					ID:          "refund-events",
					Title:       "Refund Events",
					Protocol:    "Async",
					MockHost:    "kafka://returns-control-plane.mock.internal",
					Description: "Async event surface for refund authorization callbacks and worker validation flows.",
					Operations: []APIOperation{
						{
							ID:           "publish-refund-authorized",
							Method:       "EVENT",
							Name:         "returns.refund.authorized",
							Summary:      "Publish a refund-authorized event into the worker lane.",
							ContractPath: "api/proto/refund_pricing.proto#RefundPricing",
							MockPath:     "mock/events/refund-authorized.json",
							MockURL:      "kafka://returns-control-plane.mock.internal/returns.refund.authorized",
							CurlCommand:  `curl -X POST "http://localhost:8090/mocks/async/returns-control-plane/refund-events/publish-refund-authorized?type=returns.refund.authorized" -H "content-type: application/json" -d '{"returnId":"ret_1001"}'`,
							MockMetadata: MockOperationMetadata{
								Adapter:    "async",
								RuntimeURL: "/mocks/async/returns-control-plane/refund-events/publish-refund-authorized",
								Fallback: &MockFallback{
									Mode:        "example",
									ExampleName: "authorized-event",
								},
							},
							Exchanges: []ExchangeExample{
								{
									Name:           "authorized-event",
									SourceArtifact: "mock/events/refund-authorized.json",
									When:           []MatchCondition{{From: "query", Param: "type", Value: "returns.refund.authorized"}},
									RequestHeaders: []Header{
										{Name: "content-type", Value: "application/json"},
									},
									RequestBody: `{
  "returnId": "ret_1001"
}`,
									ResponseStatus:    "202",
									ResponseMediaType: "application/json",
									ResponseHeaders: []Header{
										{Name: "x-event-stream", Value: "refunds.authorized"},
									},
									ResponseBody: `{
  "eventId": "evt_7001",
  "type": "returns.refund.authorized",
  "returnId": "ret_1001",
  "status": "published"
}`,
								},
							},
						},
					},
				},
			},
		},
		"identity-broker": {
			ID:          "identity-broker",
			Title:       "Identity Broker",
			Repository:  "localhost:5000/security/identity-broker",
			Owner:       "Security Enablement",
			Provider:    "Zot",
			Version:     "v3.0.2",
			Tags:        []string{"latest", "v3.0.2", "v3.0.1"},
			Description: "OIDC and SAML integration sandbox with deterministic login failures, cert rotation, and secret injection paths.",
			Modules:     []string{"vault", "wiremock", "postgres"},
			Status:      "Official",
			Score:       91,
			PullCommand: "babelctl run localhost:5000/security/identity-broker:v3.0.2",
			ForkCommand: "babelctl fork localhost:5000/security/identity-broker:v3.0.2 ./identity-broker-local",
			SuiteStar: `load("@babelsuite/postgres", "pg")
load("@babelsuite/runtime", "container", "mock", "script", "scenario")

broker_db = container(name="broker-db")
oidc_mock = mock(name="oidc-mock", after=["broker-db"])
saml_mock = mock(name="saml-mock", after=["broker-db"])
seed_realms = script(name="seed-realms", after=["broker-db"])
broker_api = container(name="broker-api", after=["broker-db", "oidc-mock", "saml-mock", "seed-realms"])
session_worker = container(name="session-worker", after=["broker-api"])
login_smoke = scenario(name="login-smoke", after=["broker-api", "session-worker"])`,
			Profiles: []ProfileOption{
				{FileName: "local.yaml", Label: "Local Debug", Description: "Relaxed certificates and hot-reloadable providers.", Default: true},
				{FileName: "canary.yaml", Label: "Canary", Description: "New session persistence behavior and stricter cookie policies."},
				{FileName: "ci.yaml", Label: "CI Smoke", Description: "Fast login assertions with deterministic realms."},
			},
			Folders: []FolderEntry{
				{Name: "profiles", Role: "Core", Description: "Realm, issuer, and session-storage overrides by environment.", Files: []string{"local.yaml", "canary.yaml", "ci.yaml"}},
				{Name: "api", Role: "Core", Description: "OIDC bridge OpenAPI and SAML mapping definitions.", Files: []string{"openapi/identity_broker.yaml", "proto/session.proto"}},
				{Name: "mock", Role: "Core", Description: "OIDC JWKS payloads and SAML assertion fixtures.", Files: []string{"oidc/jwks.json", "saml/assertion.xml"}},
				{Name: "scripts", Role: "Core", Description: "Realm seeders and certificate bootstrap helpers.", Files: []string{"seed_realms.ts", "rotate_certs.sh"}},
				{Name: "scenarios", Role: "Core", Description: "Login smoke suites and expired-session validation.", Files: []string{"login_smoke.py", "expired_session.py"}},
				{Name: "fixtures", Role: "Core", Description: "Realm definitions and claim payloads.", Files: []string{"claims.json", "realm_seed.yaml"}},
				{Name: "policies", Role: "Core", Description: "Cookie scope and token issuance validation.", Files: []string{"session.rego", "issuer.rego"}},
			},
			Contracts: []string{
				"Identity providers are represented as mocks with explicit dependency edges so login flows stay observable in the topology graph.",
				"Broker API containers consume db.url and signed mock endpoints from strict module return values instead of manual wiring.",
				"Scenario containers treat api/ as immutable truth and keep drift isolated to mock/ and profiles/.",
			},
			APISurfaces: []APISurface{
				{
					ID:          "broker-api",
					Title:       "Identity Broker API",
					Protocol:    "REST",
					MockHost:    "https://identity-broker.mock.internal",
					Description: "Login bridge APIs backed by OIDC and SAML mocks for deterministic browser and service flows.",
					Operations: []APIOperation{
						{
							ID:           "begin-login",
							Method:       "POST",
							Name:         "/sessions/login",
							Summary:      "Start an identity-broker login transaction.",
							ContractPath: "api/openapi/identity_broker.yaml#/paths/~1sessions~1login/post",
							MockPath:     "mock/oidc/jwks.json",
							MockURL:      "https://identity-broker.mock.internal/sessions/login?provider=oidc",
							CurlCommand:  `curl -X POST https://identity-broker.mock.internal/sessions/login?provider=oidc -H 'content-type: application/json' -d '{"email":"admin@babelsuite.test"}'`,
							Exchanges: []ExchangeExample{
								{
									Name:           "oidc-admin",
									SourceArtifact: "mock/oidc/jwks.json",
									When:           []MatchCondition{{From: "query", Param: "provider", Value: "oidc"}},
									RequestHeaders: []Header{
										{Name: "content-type", Value: "application/json"},
									},
									RequestBody: `{
  "email": "admin@babelsuite.test"
}`,
									ResponseStatus:    "200",
									ResponseMediaType: "application/json",
									ResponseHeaders: []Header{
										{Name: "set-cookie", Value: "bs_session=abc123"},
									},
									ResponseBody: `{
  "sessionId": "sess_9001",
  "provider": "oidc",
  "redirect": "https://oidc-mock.internal/authorize"
}`,
								},
								{
									Name:           "saml-fallback",
									SourceArtifact: "mock/saml/assertion.xml",
									When:           []MatchCondition{{From: "query", Param: "provider", Value: "saml"}},
									RequestHeaders: []Header{
										{Name: "content-type", Value: "application/json"},
									},
									RequestBody: `{
  "email": "ops@company.test"
}`,
									ResponseStatus:    "200",
									ResponseMediaType: "application/json",
									ResponseHeaders: []Header{
										{Name: "x-flow", Value: "saml"},
									},
									ResponseBody: `{
  "sessionId": "sess_9021",
  "provider": "saml",
  "redirect": "https://saml-mock.internal/login"
}`,
								},
							},
						},
					},
				},
			},
		},
	}
}
