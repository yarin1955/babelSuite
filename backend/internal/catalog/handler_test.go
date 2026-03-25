package catalog

import (
	"testing"

	"github.com/babelsuite/babelsuite/internal/domain"
)

func TestBuildRegistryRetainsStoredSecretsWhenEditLeavesThemBlank(t *testing.T) {
	t.Parallel()

	existing := &domain.Registry{
		RegistryID: "registry-1",
		Kind:       domain.RegistryGHCR,
		Name:       "suite-source",
		URL:        "https://ghcr.io",
		Username:   "builder",
		Password:   "secret",
	}

	reg, err := buildRegistry("org-1", existing, registryUpsertRequest{
		Name:     "suite-source",
		Kind:     "ghcr",
		URL:      "https://ghcr.io",
		Username: "builder",
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("expected update to succeed, got %v", err)
	}
	if reg.Password != "secret" {
		t.Fatalf("expected password to be retained, got %q", reg.Password)
	}
}

func TestBuildRegistryRetainsStoredTokenWhenEditLeavesItBlank(t *testing.T) {
	t.Parallel()

	existing := &domain.Registry{
		RegistryID: "registry-1",
		Kind:       domain.RegistryGHCR,
		Name:       "suite-source",
		URL:        "https://ghcr.io",
		Token:      "token-1",
	}

	reg, err := buildRegistry("org-1", existing, registryUpsertRequest{
		Name:    "suite-source",
		Kind:    "ghcr",
		URL:     "https://ghcr.io",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("expected update to succeed, got %v", err)
	}
	if reg.Token != "token-1" {
		t.Fatalf("expected bearer token to be retained, got %q", reg.Token)
	}
}

func TestValidateRegistryConnectionRejectsMixedAuthModes(t *testing.T) {
	t.Parallel()

	err := validateRegistryConnection(registryConnection{
		Kind:        domain.RegistryGHCR,
		URL:         "https://ghcr.io",
		Username:    "builder",
		Password:    "secret",
		BearerToken: "token-1",
	})
	if err == nil {
		t.Fatal("expected mixed basic auth and bearer token to be rejected")
	}
}

func TestValidateRegistryConnectionRejectsTLSOnHTTP(t *testing.T) {
	t.Parallel()

	err := validateRegistryConnection(registryConnection{
		Kind:                  domain.RegistryJFrog,
		URL:                   "http://registry.example.internal",
		InsecureSkipTLSVerify: true,
	})
	if err == nil {
		t.Fatal("expected tls settings on http registry url to be rejected")
	}
}

func TestRegistryImageHostUsesDefaultHostedRegistry(t *testing.T) {
	t.Parallel()

	host := registryImageHost(&domain.Registry{Kind: domain.RegistryGHCR})
	if host != "ghcr.io" {
		t.Fatalf("expected default hosted registry host, got %q", host)
	}
}
