package agents

import (
	"testing"

	"github.com/babelsuite/babelsuite/internal/domain"
)

func TestValidateContainerEndpoint(t *testing.T) {
	t.Parallel()

	valid := []string{
		"unix:///var/run/docker.sock",
		"npipe:////./pipe/docker_engine",
		"tcp://10.0.0.20:2376",
		"ssh://worker@example.internal",
	}

	for _, endpoint := range valid {
		if _, err := validateContainerEndpoint(endpoint); err != nil {
			t.Fatalf("expected %q to be valid, got %v", endpoint, err)
		}
	}
}

func TestValidateClusterEndpoint(t *testing.T) {
	t.Parallel()

	if _, err := validateClusterEndpoint("https://cluster.example.internal"); err != nil {
		t.Fatalf("expected cluster endpoint to be valid, got %v", err)
	}
	if _, err := validateClusterEndpoint("unix:///var/run/docker.sock"); err == nil {
		t.Fatal("expected non-http cluster endpoint to be rejected")
	}
}

func TestApplyRuntimeTargetUsesEffectiveRunnerBackend(t *testing.T) {
	t.Parallel()

	agent := &domain.Agent{}
	target := &domain.RuntimeTarget{
		RuntimeTargetID: "target-1",
		Name:            "local-builder",
		Backend:         "local",
		EndpointURL:     "",
	}

	applyRuntimeTarget(agent, target)

	if agent.DesiredBackend != "docker" {
		t.Fatalf("expected local target to map to docker runner, got %q", agent.DesiredBackend)
	}
	if agent.DesiredTargetName != "local-builder" {
		t.Fatalf("expected desired target name to be copied, got %q", agent.DesiredTargetName)
	}
}

func TestRuntimeTargetWorkerSupport(t *testing.T) {
	t.Parallel()

	if ok, reason := runtimeTargetWorkerSupport(&domain.RuntimeTarget{Backend: "docker"}); !ok || reason != "" {
		t.Fatalf("expected docker target to support workers, got ok=%v reason=%q", ok, reason)
	}
	if ok, reason := runtimeTargetWorkerSupport(&domain.RuntimeTarget{Backend: "kubernetes"}); ok || reason == "" {
		t.Fatalf("expected cluster target to be rejected for workers, got ok=%v reason=%q", ok, reason)
	}
}

func TestBuildRuntimeTargetRetainsStoredSecretsWhenEditLeavesThemBlank(t *testing.T) {
	t.Parallel()

	existing := &domain.RuntimeTarget{
		RuntimeTargetID: "target-1",
		Name:            "builder",
		Backend:         "docker",
		EndpointURL:     "tcp://10.0.0.20:2376",
		Username:        "worker",
		Password:        "secret",
	}

	target, err := buildRuntimeTarget("org-1", existing, runtimeTargetUpsertRequest{
		Name:        "builder",
		Backend:     "docker",
		EndpointURL: "tcp://10.0.0.20:2376",
		Username:    "worker",
	})
	if err != nil {
		t.Fatalf("expected update to succeed, got %v", err)
	}
	if target.Password != "secret" {
		t.Fatalf("expected password to be retained, got %q", target.Password)
	}
}

func TestValidateRuntimeTargetSecurityRejectsPasswordWithoutUsername(t *testing.T) {
	t.Parallel()

	err := validateRuntimeTarget(runtimeTargetConnection{
		Backend:     "docker",
		EndpointURL: "tcp://10.0.0.20:2376",
		Password:    "secret",
	})
	if err == nil {
		t.Fatal("expected password-only configuration to be rejected")
	}
}

func TestValidateRuntimeTargetSecurityRejectsIncompleteClientCertPair(t *testing.T) {
	t.Parallel()

	err := validateRuntimeTarget(runtimeTargetConnection{
		Backend:     "kubernetes",
		EndpointURL: "https://cluster.example.internal",
		TLSCertData: "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----",
	})
	if err == nil {
		t.Fatal("expected incomplete client certificate pair to be rejected")
	}
}
