package suites

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/babelsuite/babelsuite/internal/examplefs"
)

func TestGetReturnsClonedSuite(t *testing.T) {
	configureExamplesRoot(t)

	service := NewService()

	suite, err := service.Get("payment-suite")
	if err != nil {
		t.Fatalf("get suite: %v", err)
	}

	suite.Profiles[0].FileName = "mutated.yaml"
	suite.SourceFiles[0].Content = "mutated"

	reloaded, err := service.Get("payment-suite")
	if err != nil {
		t.Fatalf("get suite again: %v", err)
	}
	if reloaded.Profiles[0].FileName != "local.yaml" {
		t.Fatalf("expected original profile to be preserved, got %q", reloaded.Profiles[0].FileName)
	}
	if reloaded.SourceFiles[0].Content == "mutated" {
		t.Fatal("expected source files to be cloned")
	}
}

func TestListReturnsSortedSuites(t *testing.T) {
	configureExamplesRoot(t)

	service := NewService()

	items := service.List()
	if len(items) != 6 {
		t.Fatalf("expected 6 suites, got %d", len(items))
	}
	if items[0].Title != "Fleet Control Room" {
		t.Fatalf("expected sorted suites, got %q first", items[0].Title)
	}
	if items[len(items)-1].Title != "Storefront Browser Lab" {
		t.Fatalf("expected storefront browser lab last, got %q", items[len(items)-1].Title)
	}
}

func TestStorefrontSuiteHydratesSourceFiles(t *testing.T) {
	configureExamplesRoot(t)

	service := NewService()

	suite, err := service.Get("storefront-browser-lab")
	if err != nil {
		t.Fatalf("get suite: %v", err)
	}

	var found bool
	for _, file := range suite.SourceFiles {
		if file.Path != "profiles/local.yaml" {
			continue
		}
		found = true
		if file.Language != "yaml" {
			t.Fatalf("expected yaml language, got %q", file.Language)
		}
		if file.Content == "" {
			t.Fatal("expected hydrated source file content")
		}
	}

	if !found {
		t.Fatal("expected storefront source files to include profiles/local.yaml")
	}
}

func TestReturnsSuiteHydratesMockMetadataFiles(t *testing.T) {
	configureExamplesRoot(t)

	service := NewService()

	suite, err := service.Get("returns-control-plane")
	if err != nil {
		t.Fatalf("get suite: %v", err)
	}

	var found bool
	for _, file := range suite.SourceFiles {
		if file.Path != "mock/returns/create-return.metadata.yaml" {
			continue
		}
		found = true
		if file.Language != "yaml" {
			t.Fatalf("expected yaml language, got %q", file.Language)
		}
		if file.Content == "" {
			t.Fatal("expected hydrated metadata content")
		}
	}

	if !found {
		t.Fatal("expected returns suite source files to include mock metadata")
	}
}

func TestReturnsSuiteHydratesSchemaBasedMockSourceFiles(t *testing.T) {
	configureExamplesRoot(t)

	service := NewService()

	suite, err := service.Get("returns-control-plane")
	if err != nil {
		t.Fatalf("get suite: %v", err)
	}

	var found bool
	for _, file := range suite.SourceFiles {
		if file.Path != "mock/events/refund-authorized.cue" {
			continue
		}
		found = true
		if file.Language != "cue" {
			t.Fatalf("expected cue language, got %q", file.Language)
		}
		if !strings.Contains(file.Content, `requestSchema:`) {
			t.Fatalf("expected generated mock source to include requestSchema, got:\n%s", file.Content)
		}
		if !strings.Contains(file.Content, `responseSchema:`) {
			t.Fatalf("expected generated mock source to include responseSchema, got:\n%s", file.Content)
		}
	}

	if !found {
		t.Fatal("expected returns suite source files to include refund-authorized.cue")
	}
}

func TestSoapSuiteHydratesWSDLAndSchemaMockSource(t *testing.T) {
	configureExamplesRoot(t)

	service := NewService()

	suite, err := service.Get("soap-claims-hub")
	if err != nil {
		t.Fatalf("get suite: %v", err)
	}
	if len(suite.APISurfaces) == 0 || suite.APISurfaces[0].Protocol != "SOAP" {
		t.Fatalf("expected SOAP api surface, got %+v", suite.APISurfaces)
	}

	var wsdlFound bool
	var mockFound bool
	for _, file := range suite.SourceFiles {
		switch file.Path {
		case "api/wsdl/claims.wsdl":
			wsdlFound = true
			if file.Language != "xml" {
				t.Fatalf("expected wsdl to be xml, got %q", file.Language)
			}
			if !strings.Contains(file.Content, "<definitions") {
				t.Fatalf("expected generated wsdl definitions, got:\n%s", file.Content)
			}
			if !strings.Contains(file.Content, "SubmitClaim") || !strings.Contains(file.Content, "GetClaimStatus") {
				t.Fatalf("expected generated wsdl operations, got:\n%s", file.Content)
			}
		case "mock/claims/claim-service.cue":
			mockFound = true
			if file.Language != "cue" {
				t.Fatalf("expected cue mock language, got %q", file.Language)
			}
			if !strings.Contains(file.Content, "requestSchema:") || !strings.Contains(file.Content, "responseSchema:") {
				t.Fatalf("expected schema-backed soap mock source, got:\n%s", file.Content)
			}
			if !strings.Contains(file.Content, "@compose(") {
				t.Fatalf("expected soap mock schema to use cue compose rules, got:\n%s", file.Content)
			}
		}
	}

	if !wsdlFound {
		t.Fatal("expected soap suite source files to include api/wsdl/claims.wsdl")
	}
	if !mockFound {
		t.Fatal("expected soap suite source files to include mock/claims/claim-service.cue")
	}
}

func TestSuitesExposeAPISIXGatewayAndDispatcherMetadata(t *testing.T) {
	configureExamplesRoot(t)

	service := NewService()
	suite, err := service.Get("payment-suite")
	if err != nil {
		t.Fatalf("get suite: %v", err)
	}

	var gatewayFound bool
	for _, folder := range suite.Folders {
		if folder.Name != "gateway" {
			continue
		}
		for _, file := range folder.Files {
			if file == "apisix.yaml" {
				gatewayFound = true
				break
			}
		}
	}
	if !gatewayFound {
		t.Fatal("expected payment suite to expose gateway/apisix.yaml")
	}

	operation := suite.APISurfaces[0].Operations[0]
	if operation.Dispatcher != "apisix" {
		t.Fatalf("expected operation dispatcher to default to apisix, got %q", operation.Dispatcher)
	}
	if operation.MockMetadata.ResolverURL != "/internal/mock-data/payment-suite/payment-gateway/create-payment" {
		t.Fatalf("expected resolver url to be generated, got %q", operation.MockMetadata.ResolverURL)
	}
	if !strings.Contains(operation.MockMetadata.DispatcherRules, "/internal/mock-data/payment-suite/payment-gateway/create-payment") {
		t.Fatalf("expected dispatcher rules to reference resolver path, got %q", operation.MockMetadata.DispatcherRules)
	}
}

func TestSuitesGenerateAPISIXGatewaySourceWithoutUserManagedFile(t *testing.T) {
	t.Setenv(examplefs.RootEnvVar, t.TempDir())

	service := NewService()
	suite, err := service.Get("payment-suite")
	if err != nil {
		t.Fatalf("get suite: %v", err)
	}

	var found bool
	for _, file := range suite.SourceFiles {
		if file.Path != "gateway/apisix.yaml" {
			continue
		}
		found = true
		if file.Language != "yaml" {
			t.Fatalf("expected yaml language, got %q", file.Language)
		}
		if !strings.Contains(file.Content, "X-Babelsuite-Dispatcher: apisix") {
			t.Fatalf("expected generated APISIX content, got:\n%s", file.Content)
		}
		if strings.Contains(file.Content, "Missing example source") {
			t.Fatalf("expected generated APISIX content instead of missing-file placeholder, got:\n%s", file.Content)
		}
	}

	if !found {
		t.Fatal("expected payment suite source files to include generated gateway/apisix.yaml")
	}
}

func configureExamplesRoot(t *testing.T) {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current file")
	}

	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", ".."))
	t.Setenv(examplefs.RootEnvVar, filepath.Join(repoRoot, "examples"))
}
