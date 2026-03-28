package catalog

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/babelsuite/babelsuite/internal/platform"
	"github.com/babelsuite/babelsuite/internal/suites"
)

func TestListPackagesUsesConfiguredRegistryContents(t *testing.T) {
	server := registryServer(t, map[string][]string{
		"core-platform/payment-suite": {"latest", "v2.4.1", "v2.4.0"},
		"qa/unmanaged-browser-suite":  {"latest", "v0.1.0"},
		"babelsuite/kafka":            {"latest", "1.2.3"},
		"security/identity-broker":    {"latest", "v3.0.2"},
		"examples/fleet-control-room": {},
	})
	defer server.Close()

	service := NewService(suites.NewService(), stubSettingsStore{
		settings: &platform.PlatformSettings{
			Registries: []platform.OCIRegistry{
				{
					RegistryID:      "local-zot",
					Name:            "Local Zot",
					Provider:        "Zot",
					RegistryURL:     server.URL,
					RepositoryScope: "*",
				},
			},
		},
	})

	items, err := service.ListPackages()
	if err != nil {
		t.Fatalf("list packages: %v", err)
	}

	if len(items) != 4 {
		t.Fatalf("expected 4 packages from registry, got %d", len(items))
	}

	payment := findPackage(items, "payment-suite")
	if payment == nil {
		t.Fatal("expected payment-suite from registry")
	}
	if payment.Repository != server.Listener.Addr().String()+"/core-platform/payment-suite" {
		t.Fatalf("expected payment repository to use registry host, got %q", payment.Repository)
	}
	if payment.Provider != "Zot" {
		t.Fatalf("expected payment provider to come from registry, got %q", payment.Provider)
	}
	if !payment.Inspectable {
		t.Fatal("expected known suite to remain inspectable")
	}

	stdlibKafka := findPackage(items, "stdlib-kafka")
	if stdlibKafka == nil || stdlibKafka.Kind != "stdlib" {
		t.Fatal("expected stdlib kafka package from registry")
	}

	generic := findPackage(items, packageID(server.Listener.Addr().String()+"/qa/unmanaged-browser-suite", "suite"))
	if generic == nil {
		t.Fatal("expected unmanaged registry package to appear in catalog")
	}
	if !generic.Inspectable {
		t.Fatal("expected unmanaged registry suite to expose the inspect action")
	}
	if generic.Title != "Unmanaged Browser Suite" {
		t.Fatalf("expected humanized title, got %q", generic.Title)
	}
}

func TestGetPackageReturnsRegistryBackedSuite(t *testing.T) {
	server := registryServer(t, map[string][]string{
		"core-platform/payment-suite": {"latest", "v2.4.1"},
	})
	defer server.Close()

	service := NewService(suites.NewService(), stubSettingsStore{
		settings: &platform.PlatformSettings{
			Registries: []platform.OCIRegistry{
				{
					RegistryID:      "local-zot",
					Name:            "Local Zot",
					Provider:        "Zot",
					RegistryURL:     server.URL,
					RepositoryScope: "*",
				},
			},
		},
	})

	item, err := service.GetPackage("payment-suite")
	if err != nil {
		t.Fatalf("get package: %v", err)
	}
	if item.Kind != "suite" {
		t.Fatalf("expected suite kind, got %q", item.Kind)
	}
	if item.Provider != "Zot" {
		t.Fatalf("expected registry-backed provider, got %q", item.Provider)
	}
	if item.Version != "v2.4.1" {
		t.Fatalf("expected registry version, got %q", item.Version)
	}
}

type stubSettingsStore struct {
	settings *platform.PlatformSettings
}

func (s stubSettingsStore) Load() (*platform.PlatformSettings, error) {
	return s.settings, nil
}

func (s stubSettingsStore) Save(_ *platform.PlatformSettings) error {
	return nil
}

func (s stubSettingsStore) SyncRegistry(_ string) (*platform.PlatformSettings, error) {
	return s.settings, nil
}

func registryServer(t *testing.T, repositories map[string][]string) *httptest.Server {
	t.Helper()

	handler := http.NewServeMux()
	handler.HandleFunc("/v2/_catalog", func(w http.ResponseWriter, _ *http.Request) {
		names := make([]string, 0, len(repositories))
		for name := range repositories {
			names = append(names, name)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"repositories": names})
	})

	handler.HandleFunc("/v2/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if !strings.HasSuffix(path, "/tags/list") {
			http.NotFound(w, r)
			return
		}

		repository := strings.TrimPrefix(strings.TrimSuffix(path, "/tags/list"), "/v2/")
		tags, ok := repositories[repository]
		if !ok {
			http.NotFound(w, r)
			return
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": repository,
			"tags": tags,
		})
	})

	return httptest.NewServer(handler)
}

func findPackage(items []Package, id string) *Package {
	for _, item := range items {
		if item.ID == id {
			clone := item
			return &clone
		}
	}
	return nil
}
