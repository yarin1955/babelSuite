package execution

import "time"

func (s *Service) seedHistory() {
	s.seedHistoricalExecution("run-1043", "payment-suite", "staging.yaml", "Manual", "Healthy", 2*time.Minute)
	s.seedHistoricalExecution("run-1042", "fleet-control-room", "perf.yaml", "CI", "Healthy", 7*time.Minute)
	s.seedHistoricalExecution("run-1041", "identity-broker", "ci.yaml", "CI", "Failed", 24*time.Minute)
}

func (s *Service) seedHistoricalExecution(executionID, suiteID, profile, trigger, status string, startedAgo time.Duration) {
	suite, err := s.suiteSource.Get(suiteID)
	if err != nil {
		return
	}
	meta := s.suiteMeta[suiteID]

	startedAt := time.Now().UTC().Add(-startedAgo)
	topology := parseSuiteTopologyOrEmpty(suite.SuiteStar)
	events := buildHistoricalEvents(suite, topology, status, profile, meta)

	state := &executionState{
		record: ExecutionRecord{
			ID:        executionID,
			Suite:     buildExecutionSuite(*suite),
			Profile:   profile,
			Trigger:   trigger,
			Status:    status,
			StartedAt: startedAt,
			UpdatedAt: startedAt.Add(time.Duration(len(events)) * 2 * time.Second),
			Author:    meta.Author,
			Commit:    buildCommitHash(suite.ID, executionID),
			Branch:    meta.Branch,
			Message:   meta.Message,
			Events:    events,
		},
		total:     len(topology),
		completed: len(topology),
	}

	if status == "Failed" {
		state.completed = max(len(topology)-1, 0)
	}

	s.executions[executionID] = state
	s.order = append(s.order, executionID)
	s.logs.Open(executionID)
	for _, event := range events {
		s.appendLog(executionID, event)
	}
	go s.syncObservers(executionID)
}

func seedExecutionMetadata() map[string]suiteRuntimeMeta {
	return map[string]suiteRuntimeMeta{
		"payment-suite": {
			Author:         "Nora Chen",
			Branch:         "release/payments-v2",
			Message:        "Promote the payment environment with deterministic fraud routing and wiremock-backed checkout paths.",
			FailureTarget:  "checkout-smoke",
			DefaultTrigger: "Manual",
		},
		"fleet-control-room": {
			Author:         "Ari Levin",
			Branch:         "feature/planner-observability",
			Message:        "Run the fleet control topology against mocked telemetry bursts before planner rollout.",
			FailureTarget:  "fleet-smoke",
			DefaultTrigger: "Manual",
		},
		"identity-broker": {
			Author:         "Sam Okafor",
			Branch:         "canary/session-cache",
			Message:        "Validate multi-provider login behavior with strict mock assertions and seeded realms.",
			FailureTarget:  "login-smoke",
			DefaultTrigger: "CI",
		},
		"storefront-browser-lab": {
			Author:         "Lena Hart",
			Branch:         "feature/browser-checkout-lab",
			Message:        "Exercise the storefront browser lane with Playwright, mock APIs, and Kafka-backed checkout events.",
			FailureTarget:  "playwright-checkout",
			DefaultTrigger: "Manual",
		},
		"soap-claims-hub": {
			Author:         "Maya Ortiz",
			Branch:         "feature/soap-bridge-modernization",
			Message:        "Validate SOAP envelopes through the APISIX sidecar before exposing the partner claims bridge.",
			FailureTarget:  "claims-smoke",
			DefaultTrigger: "Manual",
		},
	}
}
