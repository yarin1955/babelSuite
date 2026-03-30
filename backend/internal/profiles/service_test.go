package profiles

import (
	"testing"

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

func TestServicePreventsDeletingBaseProfile(t *testing.T) {
	service := NewService(suites.NewService(), NewMemoryStore())

	_, err := service.DeleteProfile("payment-suite", "base")
	if err == nil {
		t.Fatal("expected deleting base profile to fail")
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
