package support

import (
	"testing"

	"github.com/babelsuite/babelsuite/pkg/apiclient"
)

func TestSplitReference(t *testing.T) {
	repository, version := SplitReference("localhost:5000/qa/storefront-browser-lab:v1.3.0")
	if repository != "localhost:5000/qa/storefront-browser-lab" {
		t.Fatalf("unexpected repository: %s", repository)
	}
	if version != "v1.3.0" {
		t.Fatalf("unexpected version: %s", version)
	}
}

func TestResolveLaunchTargetByRepositoryPath(t *testing.T) {
	item, err := ResolveLaunchTarget("qa/storefront-browser-lab", []apiclient.LaunchSuite{
		{ID: "storefront-browser-lab", Repository: "localhost:5000/qa/storefront-browser-lab"},
	})
	if err != nil {
		t.Fatalf("ResolveLaunchTarget returned error: %v", err)
	}
	if item.ID != "storefront-browser-lab" {
		t.Fatalf("unexpected suite id: %s", item.ID)
	}
}

func TestResolveCatalogTargetByFullReference(t *testing.T) {
	item, err := ResolveCatalogTarget("localhost:5000/core-platform/payment-suite:v2.4.1", []apiclient.CatalogPackage{
		{ID: "payment-suite", Repository: "localhost:5000/core-platform/payment-suite"},
	})
	if err != nil {
		t.Fatalf("ResolveCatalogTarget returned error: %v", err)
	}
	if item.ID != "payment-suite" {
		t.Fatalf("unexpected package id: %s", item.ID)
	}
}

func TestWriteSuiteFilesRejectsTraversal(t *testing.T) {
	_, err := WriteSuiteFiles(t.TempDir(), []apiclient.SuiteSourceFile{
		{Path: "../escape.txt", Content: "bad"},
	}, false)
	if err == nil {
		t.Fatal("expected traversal path to fail")
	}
}
