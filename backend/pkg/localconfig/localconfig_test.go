package localconfig

import (
	"path/filepath"
	"testing"
)

func TestStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	store := NewStore(path)

	if err := store.Save(&Config{
		Server:    "http://localhost:8090",
		Token:     "token",
		Email:     "admin@babelsuite.test",
		Workspace: "Admin workspace",
	}); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	cfg, err := store.Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.Server != "http://localhost:8090" {
		t.Fatalf("unexpected server: %s", cfg.Server)
	}
	if cfg.Token != "token" {
		t.Fatalf("unexpected token: %s", cfg.Token)
	}
}
