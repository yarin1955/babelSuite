package profiles

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/babelsuite/babelsuite/internal/demofs"
	"github.com/babelsuite/babelsuite/internal/examplefs"
	"github.com/babelsuite/babelsuite/internal/suites"
)

func TestServiceExposesSuiteScopedLaunchProfiles(t *testing.T) {
	service := NewService(suites.NewService(), NewMemoryStore())

	paymentSuite, err := service.Get("payment-suite")
	if err != nil {
		t.Fatalf("get payment suite: %v", err)
	}
	if len(paymentSuite.Profiles) != 3 {
		t.Fatalf("expected 3 launchable payment profiles, got %d", len(paymentSuite.Profiles))
	}
	if containsProfile(paymentSuite.Profiles, "perf.yaml") {
		t.Fatal("did not expect fleet profile to appear in payment suite")
	}
	if !containsProfile(paymentSuite.Profiles, "year.yaml") {
		t.Fatal("expected payment suite to expose year.yaml")
	}
}

func TestServiceCreatesProfileAndSetsDefault(t *testing.T) {
	service := NewService(suites.NewService(), NewMemoryStore())

	created, err := service.CreateProfile("payment-suite", UpsertRequest{
		Name:        "Holiday Freeze",
		FileName:    "holiday.yaml",
		Description: "Freeze routing for end-of-quarter reconciliation.",
		Scope:       "Staging",
		YAML:        "env:\n  LEDGER_PERIOD: holiday\nservices:\n  workerReplicaCount: 2\n",
		SecretRefs: []SecretReference{
			{Key: "FREEZE_TOKEN", Provider: "Vault", Ref: "kv/payment-suite/holiday-freeze-token"},
		},
		ExtendsID: "base",
	})
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if !suiteContainsProfile(created, "holiday.yaml") {
		t.Fatal("expected holiday.yaml to be persisted")
	}

	holidayID := findProfileID(created, "holiday.yaml")
	updated, err := service.SetDefaultProfile("payment-suite", holidayID)
	if err != nil {
		t.Fatalf("set default profile: %v", err)
	}
	if updated.DefaultProfileFileName != "holiday.yaml" {
		t.Fatalf("expected holiday.yaml to become default, got %q", updated.DefaultProfileFileName)
	}
}

func TestServiceExtractsSecretRefsFromManagedProfileYAML(t *testing.T) {
	service := NewService(suites.NewService(), NewMemoryStore())

	created, err := service.CreateProfile("payment-suite", UpsertRequest{
		Name:        "Direct YAML",
		FileName:    "direct-yaml.yaml",
		Description: "Inline secret refs live in the YAML body.",
		Scope:       "Staging",
		YAML: `secretRefs:
  - key: API_TOKEN
    provider: Vault
    ref: kv/payment-suite/direct-yaml-token
env:
  LOG_LEVEL: debug`,
	})
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}

	record := findProfileRecordByFileName(created, "direct-yaml.yaml")
	if record == nil {
		t.Fatal("expected direct-yaml.yaml to be returned")
	}
	if len(record.SecretRefs) != 1 {
		t.Fatalf("expected 1 secret ref extracted from yaml, got %d", len(record.SecretRefs))
	}
	if got := record.SecretRefs[0]; got.Key != "API_TOKEN" || got.Provider != "Vault" || got.Ref != "kv/payment-suite/direct-yaml-token" {
		t.Fatalf("unexpected extracted secret ref: %#v", got)
	}
}

func TestServicePreventsDeletingBaseProfile(t *testing.T) {
	service := NewService(suites.NewService(), NewMemoryStore())

	_, err := service.DeleteProfile("payment-suite", "base")
	if err == nil {
		t.Fatal("expected deleting base profile to fail")
	}
}

func TestServiceLoadsWorkspaceProfilesWhenDemoDisabled(t *testing.T) {
	t.Setenv(demofs.EnableEnvVar, "false")
	configureProfilesExamplesRoot(t)

	service := NewService(suites.NewService(), NewMemoryStore())
	paymentSuite, err := service.Get("payment-suite")
	if err != nil {
		t.Fatalf("get payment suite: %v", err)
	}
	if len(paymentSuite.Profiles) == 0 {
		t.Fatal("expected workspace launch profiles when demo is disabled")
	}

	profilesPayload, err := service.GetSuiteProfiles("payment-suite")
	if err != nil {
		t.Fatalf("get suite profiles: %v", err)
	}
	if !suiteContainsProfile(profilesPayload, "local.yaml") {
		t.Fatal("expected local.yaml in workspace-backed profiles")
	}
	staging := findProfileRecordByFileName(profilesPayload, "staging.yaml")
	if staging == nil {
		t.Fatal("expected staging.yaml in workspace-backed profiles")
	}
	if len(staging.SecretRefs) != 1 {
		t.Fatalf("expected 1 inline secret ref in staging.yaml, got %d", len(staging.SecretRefs))
	}
	if got := staging.SecretRefs[0]; got.Key != "DB_PASSWORD" || got.Provider != "Vault" || got.Ref != "kv/payment-suite/staging-db-password" {
		t.Fatalf("unexpected workspace secret ref: %#v", got)
	}
}

func containsProfile(profiles []suites.ProfileOption, fileName string) bool {
	for _, profile := range profiles {
		if profile.FileName == fileName {
			return true
		}
	}
	return false
}

func suiteContainsProfile(payload *SuiteProfiles, fileName string) bool {
	for _, profile := range payload.Profiles {
		if profile.FileName == fileName {
			return true
		}
	}
	return false
}

func findProfileID(payload *SuiteProfiles, fileName string) string {
	for _, profile := range payload.Profiles {
		if profile.FileName == fileName {
			return profile.ID
		}
	}
	return ""
}

func findProfileRecordByFileName(payload *SuiteProfiles, fileName string) *Record {
	for index := range payload.Profiles {
		if payload.Profiles[index].FileName == fileName {
			return &payload.Profiles[index]
		}
	}
	return nil
}

func configureProfilesExamplesRoot(t *testing.T) {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current file")
	}

	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", ".."))
	t.Setenv(examplefs.RootEnvVar, filepath.Join(repoRoot, "examples"))
}
