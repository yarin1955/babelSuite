package auth

import (
	"context"
	"testing"

	"github.com/babelsuite/babelsuite/internal/domain"
)

func TestSeedCreatesAdminUserWithEmail(t *testing.T) {
	t.Parallel()

	stub := newStubStore()

	Seed(context.Background(), stub, "admin@babelsuite.test", "admin")

	user, err := stub.GetUserByEmail(context.Background(), "admin@babelsuite.test")
	if err != nil {
		t.Fatalf("expected seeded admin user: %v", err)
	}
	if user.Email != "admin@babelsuite.test" {
		t.Fatalf("unexpected email: %q", user.Email)
	}
	if user.Username != "admin" {
		t.Fatalf("unexpected username: %q", user.Username)
	}
	if !user.IsAdmin {
		t.Fatal("expected seeded user to be admin")
	}
	if user.WorkspaceID == "" {
		t.Fatal("expected seeded user to have a workspace")
	}
}

func TestSeedRetriesUsernameWhenLegacyAdminExists(t *testing.T) {
	t.Parallel()

	stub := newStubStore()
	legacyWorkspace := &domain.Workspace{
		WorkspaceID: "workspace-legacy",
		Slug:        "admin",
		Name:        "Legacy admin workspace",
	}
	legacyUser := &domain.User{
		UserID:      "user-legacy",
		WorkspaceID: legacyWorkspace.WorkspaceID,
		Username:    "admin",
		Email:       "admin",
		FullName:    "Legacy Admin",
		IsAdmin:     true,
		PassHash:    "legacy",
	}
	if err := stub.CreateWorkspace(context.Background(), legacyWorkspace); err != nil {
		t.Fatalf("seed legacy workspace: %v", err)
	}
	if err := stub.CreateUser(context.Background(), legacyUser); err != nil {
		t.Fatalf("seed legacy user: %v", err)
	}

	Seed(context.Background(), stub, "admin@babelsuite.test", "admin")

	user, err := stub.GetUserByEmail(context.Background(), "admin@babelsuite.test")
	if err != nil {
		t.Fatalf("expected new email-based admin user: %v", err)
	}
	if user.Username == "admin" {
		t.Fatal("expected seed to avoid conflicting legacy username")
	}
	if !user.IsAdmin {
		t.Fatal("expected seeded user to be admin")
	}
}
