package catalog

import (
	"testing"

	"github.com/babelsuite/babelsuite/internal/suites"
)

func TestListPackagesIncludesSuitesAndStdlib(t *testing.T) {
	service := NewService(suites.NewService())

	items := service.ListPackages()
	if len(items) != 5 {
		t.Fatalf("expected 5 packages, got %d", len(items))
	}

	foundStdlib := false
	foundSuite := false
	for _, item := range items {
		if item.Kind == "stdlib" {
			foundStdlib = true
		}
		if item.ID == "payment-suite" && item.Kind == "suite" {
			foundSuite = true
		}
	}

	if !foundStdlib || !foundSuite {
		t.Fatalf("expected both suite and stdlib packages in list")
	}
}

func TestGetPackageReturnsSuitePackage(t *testing.T) {
	service := NewService(suites.NewService())

	item, err := service.GetPackage("payment-suite")
	if err != nil {
		t.Fatalf("get package: %v", err)
	}
	if item.Kind != "suite" {
		t.Fatalf("expected suite kind, got %q", item.Kind)
	}
	if item.Title != "Payment Suite" {
		t.Fatalf("expected payment suite title, got %q", item.Title)
	}
}
