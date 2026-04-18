package execution

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/babelsuite/babelsuite/internal/demofs"
	"github.com/babelsuite/babelsuite/internal/examplefs"
	"github.com/babelsuite/babelsuite/internal/platform"
	"github.com/babelsuite/babelsuite/internal/profiles"
	"github.com/babelsuite/babelsuite/internal/suites"
)

func TestRunNodeInjectsManagedProfileSecretsAndOverridesIntoBackend(t *testing.T) {
	t.Setenv(demofs.EnableEnvVar, "false")
	t.Setenv("BABELSUITE_VAULT_TOKEN", "vault-token")

	vaultServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/kv/platform/service/api" {
			t.Fatalf("unexpected vault path %q", r.URL.Path)
		}
		if got := r.Header.Get("X-Vault-Token"); got != "vault-token" {
			t.Fatalf("expected vault token header, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"data":{"token":"vault-secret-token"}}}`))
	}))
	defer vaultServer.Close()

	source := staticSuiteSource{
		items: map[string]suites.Definition{
			"secret-suite": {
				ID:         "secret-suite",
				Title:      "Secret Suite",
				Repository: "localhost:5000/testing/secret-suite",
				SuiteStar: strings.Join([]string{
					`api = service.run(name="api")`,
					`worker = service.run(name="worker", after=[api])`,
				}, "\n"),
				Profiles: []suites.ProfileOption{
					{FileName: "local.yaml", Label: "Local", Default: true},
				},
			},
		},
	}

	profileService := profiles.NewService(source, profiles.NewMemoryStore())
	if _, err := profileService.CreateProfile("secret-suite", profiles.UpsertRequest{
		Name:        "Staging",
		FileName:    "staging.yaml",
		Description: "Stage secrets and runtime overrides.",
		Scope:       "Staging",
		YAML: strings.TrimSpace(`
env:
  LOG_LEVEL: debug
  SHARED_SETTING: profile
services:
  api:
    env:
      API_MODE: strict
`),
		SecretRefs: []profiles.SecretReference{
			{Key: "API_TOKEN", Provider: "Vault", Ref: "service/api#token"},
			{Key: "TELEMETRY_TOKEN", Provider: "Local Secret", Ref: "secrets://developer/telemetry-token"},
		},
		ExtendsID: "local",
	}); err != nil {
		t.Fatalf("create staging profile: %v", err)
	}

	service := NewServiceWithPlatform(profileService, stubPlatformSource{
		settings: &platform.PlatformSettings{
			Agents: []platform.ExecutionAgent{
				{
					AgentID: "local-docker",
					Name:    "Local Docker",
					Type:    "local",
					Enabled: true,
					Default: true,
				},
			},
			Secrets: platform.SecretsConfig{
				Provider:       "vault",
				VaultAddress:   vaultServer.URL,
				VaultNamespace: "platform",
				SecretPrefix:   "kv/platform",
				GlobalOverrides: []platform.GlobalOverride{
					{Key: "HTTPS_PROXY", Value: "http://proxy.internal.company.com:8080"},
					{Key: "TELEMETRY_TOKEN", Value: "local-secret-token"},
				},
			},
		},
	})
	defer service.Close()

	suite, err := profileService.Get("secret-suite")
	if err != nil {
		t.Fatalf("get secret suite: %v", err)
	}
	resolved, err := suites.ResolveRuntime(*suite, profileService.List())
	if err != nil {
		t.Fatalf("resolve runtime: %v", err)
	}
	suite.Topology = resolved.Nodes

	overlay, err := service.resolveExecutionRuntimeOverlay(context.Background(), suite.ID, "staging.yaml")
	if err != nil {
		t.Fatalf("resolve runtime overlay: %v", err)
	}

	now := time.Now().UTC()
	state := &executionState{
		record: ExecutionRecord{
			ID:        "run-secret",
			Suite:     buildExecutionSuite(*suite),
			Profile:   "staging.yaml",
			BackendID: "capture",
			Backend:   "Capture",
			Trigger:   "Manual",
			Status:    "Booting",
			StartedAt: now,
			UpdatedAt: now,
		},
		runtime:    overlay,
		total:      len(suite.Topology),
		stepStatus: map[string]string{},
	}
	for _, node := range suite.Topology {
		state.stepStatus[node.ID] = "pending"
	}

	service.mu.Lock()
	service.executions["run-secret"] = state
	service.mu.Unlock()
	service.logs.Open("run-secret")

	backend := &captureBackend{}
	var apiNode topologyNode
	for _, node := range suite.Topology {
		if node.ID == "api" {
			apiNode = node
			break
		}
	}

	if err := service.runNode(context.Background(), "run-secret", suite, "staging.yaml", backend, apiNode); err != nil {
		t.Fatalf("run node: %v", err)
	}

	if got := backend.spec.Env["LOG_LEVEL"]; got != "debug" {
		t.Fatalf("expected LOG_LEVEL from profile yaml, got %q", got)
	}
	if got := backend.spec.Env["HTTPS_PROXY"]; got != "http://proxy.internal.company.com:8080" {
		t.Fatalf("expected HTTPS_PROXY global override, got %q", got)
	}
	if got := backend.spec.Env["API_MODE"]; got != "strict" {
		t.Fatalf("expected API_MODE service override, got %q", got)
	}
	if got := backend.spec.Env["API_TOKEN"]; got != "vault-secret-token" {
		t.Fatalf("expected API_TOKEN from vault secret ref, got %q", got)
	}
	if got := backend.spec.Env["TELEMETRY_TOKEN"]; got != "local-secret-token" {
		t.Fatalf("expected TELEMETRY_TOKEN from local secret/global override, got %q", got)
	}
	if got := backend.spec.RuntimeProfile; got != "staging.yaml" {
		t.Fatalf("expected runtime profile staging.yaml, got %q", got)
	}

	for _, node := range state.record.Suite.Topology {
		if node.ID != "api" {
			continue
		}
		if _, ok := node.RuntimeEnv["API_TOKEN"]; ok {
			t.Fatal("did not expect resolved secrets to be persisted into execution topology metadata")
		}
		if _, ok := node.RuntimeEnv["HTTPS_PROXY"]; ok {
			t.Fatal("did not expect global overrides to be persisted into execution topology metadata")
		}
	}
}

func TestRunNodeInjectsPaymentSuiteStagingProfileRuntimeIntoBackend(t *testing.T) {
	profileService := profiles.NewService(suites.NewService(), profiles.NewMemoryStore())
	service := NewService(profileService)
	defer service.Close()

	suite, err := profileService.Get("payment-suite")
	if err != nil {
		t.Fatalf("get payment suite: %v", err)
	}
	resolved, err := suites.ResolveRuntime(*suite, profileService.List())
	if err != nil {
		t.Fatalf("resolve runtime: %v", err)
	}
	suite.Topology = resolved.Nodes

	overlay, err := service.resolveExecutionRuntimeOverlay(context.Background(), suite.ID, "staging.yaml")
	if err != nil {
		t.Fatalf("resolve runtime overlay: %v", err)
	}

	now := time.Now().UTC()
	state := &executionState{
		record: ExecutionRecord{
			ID:        "run-payment-staging",
			Suite:     buildExecutionSuite(*suite),
			Profile:   "staging.yaml",
			BackendID: "capture",
			Backend:   "Capture",
			Trigger:   "Manual",
			Status:    "Booting",
			StartedAt: now,
			UpdatedAt: now,
		},
		runtime:    overlay,
		total:      len(suite.Topology),
		stepStatus: map[string]string{},
	}
	for _, node := range suite.Topology {
		state.stepStatus[node.ID] = "pending"
	}

	service.mu.Lock()
	service.executions["run-payment-staging"] = state
	service.mu.Unlock()
	service.logs.Open("run-payment-staging")

	backend := &captureBackend{}
	var gatewayNode topologyNode
	for _, node := range suite.Topology {
		if node.ID == "payment_gateway" {
			gatewayNode = node
			break
		}
	}
	if gatewayNode.ID == "" {
		t.Fatal("expected payment_gateway node in payment-suite topology")
	}

	if err := service.runNode(context.Background(), "run-payment-staging", suite, "staging.yaml", backend, gatewayNode); err != nil {
		t.Fatalf("run node: %v", err)
	}

	if got := backend.spec.Env["LOG_LEVEL"]; got != "info" {
		t.Fatalf("expected inherited LOG_LEVEL from base profile, got %q", got)
	}
	if got := backend.spec.Env["PAYMENTS_API_BASE_URL"]; got != "https://payments.staging.company.test" {
		t.Fatalf("expected PAYMENTS_API_BASE_URL from staging profile, got %q", got)
	}
	if got := backend.spec.Env["FRAUD_STRATEGY"]; got != "strict" {
		t.Fatalf("expected FRAUD_STRATEGY from staging profile, got %q", got)
	}
	if got := backend.spec.Env["POSTGRES_URL"]; got != "postgres://test:test@db:5432/payments" {
		t.Fatalf("expected inherited POSTGRES_URL from base service env, got %q", got)
	}
	if got := backend.spec.Env["API_PORT"]; got != "8080" {
		t.Fatalf("expected API_PORT from staging service env, got %q", got)
	}
	if got := backend.spec.Env["ROUTING_MODE"]; got != "staging" {
		t.Fatalf("expected ROUTING_MODE from staging service env, got %q", got)
	}
	if _, ok := backend.spec.Env["JWT_PRIVATE_KEY"]; ok {
		t.Fatal("did not expect unresolved base-profile secrets to be injected without platform settings")
	}
	if _, ok := backend.spec.Env["DB_PASSWORD"]; ok {
		t.Fatal("did not expect unresolved staging-profile secrets to be injected without platform settings")
	}
}

func TestRunNodeInjectsWorkspaceProfileInlineSecretRefsIntoBackend(t *testing.T) {
	t.Setenv(demofs.EnableEnvVar, "false")
	t.Setenv("BABELSUITE_VAULT_TOKEN", "vault-token")
	configureExecutionExamplesRoot(t)

	vaultServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/kv/payment-suite/staging-db-password" {
			t.Fatalf("unexpected vault path %q", r.URL.Path)
		}
		if got := r.Header.Get("X-Vault-Token"); got != "vault-token" {
			t.Fatalf("expected vault token header, got %q", got)
		}
		if got := r.Header.Get("X-Vault-Namespace"); got != "payments" {
			t.Fatalf("expected vault namespace header, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"data":{"DB_PASSWORD":"workspace-db-password"}}}`))
	}))
	defer vaultServer.Close()

	profileService := profiles.NewService(suites.NewService(), profiles.NewMemoryStore())
	service := NewServiceWithPlatform(profileService, stubPlatformSource{
		settings: &platform.PlatformSettings{
			Agents: []platform.ExecutionAgent{
				{
					AgentID: "local-docker",
					Name:    "Local Docker",
					Type:    "local",
					Enabled: true,
					Default: true,
				},
			},
			Secrets: platform.SecretsConfig{
				Provider:       "vault",
				VaultAddress:   vaultServer.URL,
				VaultNamespace: "payments",
				SecretPrefix:   "kv/platform",
			},
		},
	})
	defer service.Close()

	suite, err := profileService.Get("payment-suite")
	if err != nil {
		t.Fatalf("get payment suite: %v", err)
	}
	resolved, err := suites.ResolveRuntime(*suite, profileService.List())
	if err != nil {
		t.Fatalf("resolve runtime: %v", err)
	}
	suite.Topology = resolved.Nodes

	overlay, err := service.resolveExecutionRuntimeOverlay(context.Background(), suite.ID, "staging.yaml")
	if err != nil {
		t.Fatalf("resolve runtime overlay: %v", err)
	}

	now := time.Now().UTC()
	state := &executionState{
		record: ExecutionRecord{
			ID:        "run-payment-workspace-staging",
			Suite:     buildExecutionSuite(*suite),
			Profile:   "staging.yaml",
			BackendID: "capture",
			Backend:   "Capture",
			Trigger:   "Manual",
			Status:    "Booting",
			StartedAt: now,
			UpdatedAt: now,
		},
		runtime:    overlay,
		total:      len(suite.Topology),
		stepStatus: map[string]string{},
	}
	for _, node := range suite.Topology {
		state.stepStatus[node.ID] = "pending"
	}

	service.mu.Lock()
	service.executions["run-payment-workspace-staging"] = state
	service.mu.Unlock()
	service.logs.Open("run-payment-workspace-staging")

	backend := &captureBackend{}
	var gatewayNode topologyNode
	for _, node := range suite.Topology {
		if node.ID == "payment_gateway" {
			gatewayNode = node
			break
		}
	}
	if gatewayNode.ID == "" {
		t.Fatal("expected payment_gateway node in payment-suite topology")
	}

	if err := service.runNode(context.Background(), "run-payment-workspace-staging", suite, "staging.yaml", backend, gatewayNode); err != nil {
		t.Fatalf("run node: %v", err)
	}

	if got := backend.spec.Env["PAYMENTS_API_BASE_URL"]; got != "https://payments.staging.company.test" {
		t.Fatalf("expected PAYMENTS_API_BASE_URL from workspace staging profile, got %q", got)
	}
	if got := backend.spec.Env["FRAUD_STRATEGY"]; got != "strict" {
		t.Fatalf("expected FRAUD_STRATEGY from workspace staging profile, got %q", got)
	}
	if got := backend.spec.Env["API_PORT"]; got != "8080" {
		t.Fatalf("expected API_PORT from workspace service env, got %q", got)
	}
	if got := backend.spec.Env["ROUTING_MODE"]; got != "staging" {
		t.Fatalf("expected ROUTING_MODE from workspace service env, got %q", got)
	}
	if got := backend.spec.Env["DB_PASSWORD"]; got != "workspace-db-password" {
		t.Fatalf("expected DB_PASSWORD from workspace inline secret ref, got %q", got)
	}
}

func TestResolveExecutionRuntimeOverlaySkipsWorkspaceVaultRefsWhenPlatformDefaultsDoNotEnableVault(t *testing.T) {
	t.Setenv(demofs.EnableEnvVar, "false")
	configureExecutionExamplesRoot(t)

	profileService := profiles.NewService(suites.NewService(), profiles.NewMemoryStore())
	settings := platform.DefaultSettings()
	service := NewServiceWithPlatform(profileService, stubPlatformSource{settings: &settings})
	defer service.Close()

	overlay, err := service.resolveExecutionRuntimeOverlay(context.Background(), "payment-suite", "staging.yaml")
	if err != nil {
		t.Fatalf("resolve runtime overlay: %v", err)
	}

	if got := overlay.Env["PAYMENTS_API_BASE_URL"]; got != "https://payments.staging.company.test" {
		t.Fatalf("expected PAYMENTS_API_BASE_URL from workspace staging profile, got %q", got)
	}
	if got := overlay.Env["HTTPS_PROXY"]; got != "http://proxy.internal.company.com:8080" {
		t.Fatalf("expected global override to remain available, got %q", got)
	}
	if _, ok := overlay.SecretEnv["DB_PASSWORD"]; ok {
		t.Fatal("did not expect Vault-backed DB_PASSWORD to resolve when platform defaults leave Vault disabled")
	}
}

func configureExecutionExamplesRoot(t *testing.T) {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current file")
	}

	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", ".."))
	t.Setenv(examplefs.RootEnvVar, filepath.Join(repoRoot, "examples"))
}
