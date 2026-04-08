package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/babelsuite/babelsuite/internal/domain"
	"github.com/babelsuite/babelsuite/internal/store"
	"golang.org/x/crypto/bcrypt"
)

func TestSignUpCreatesWorkspaceAndUser(t *testing.T) {
	t.Parallel()

	stub := newStubStore()
	handler := NewHandler(stub, NewJWT("test-secret"), testAuthConfig())

	body := bytes.NewBufferString(`{"fullName":"Ada Lovelace","email":"ada@example.com","password":"Sup3rStrong!"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sign-up", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.signUp(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, rec.Code)
	}

	var response authResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Token == "" {
		t.Fatal("expected token to be returned")
	}
	if response.User == nil || response.User.Email != "ada@example.com" {
		t.Fatalf("unexpected user payload: %+v", response.User)
	}
	if response.Workspace == nil || response.Workspace.Name != "Ada's workspace" {
		t.Fatalf("unexpected workspace payload: %+v", response.Workspace)
	}
}

func TestSignInRejectsWrongPassword(t *testing.T) {
	t.Parallel()

	hash, err := bcrypt.GenerateFromPassword([]byte("right-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	stub := newStubStore()
	workspace := &domain.Workspace{
		WorkspaceID: "workspace-1",
		Slug:        "ada-workspace",
		Name:        "Ada's workspace",
		CreatedAt:   time.Now().UTC(),
	}
	user := &domain.User{
		UserID:      "user-1",
		WorkspaceID: workspace.WorkspaceID,
		Username:    "ada",
		Email:       "ada@example.com",
		FullName:    "Ada Lovelace",
		PassHash:    string(hash),
		CreatedAt:   time.Now().UTC(),
	}
	if err := stub.CreateWorkspace(context.Background(), workspace); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := stub.CreateUser(context.Background(), user); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	handler := NewHandler(stub, NewJWT("test-secret"), testAuthConfig())
	body := bytes.NewBufferString(`{"email":"ada@example.com","password":"wrong-password"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sign-in", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.signIn(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestSignInRejectsWhenPasswordAuthDisabled(t *testing.T) {
	t.Parallel()

	handler := NewHandler(newStubStore(), NewJWT("test-secret"), Config{
		PasswordAuthEnabled: false,
		SignUpEnabled:       true,
	})

	body := bytes.NewBufferString(`{"email":"ada@example.com","password":"right-password"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sign-in", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.signIn(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, rec.Code)
	}
}

func TestMeUsesAdminClaimFromToken(t *testing.T) {
	t.Parallel()

	stub := newStubStore()
	workspace := &domain.Workspace{
		WorkspaceID: "workspace-1",
		Slug:        "ada-workspace",
		Name:        "Ada's workspace",
		CreatedAt:   time.Now().UTC(),
	}
	user := &domain.User{
		UserID:      "user-1",
		WorkspaceID: workspace.WorkspaceID,
		Username:    "ada",
		Email:       "ada@example.com",
		FullName:    "Ada Lovelace",
		IsAdmin:     false,
		CreatedAt:   time.Now().UTC(),
	}
	if err := stub.CreateWorkspace(context.Background(), workspace); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := stub.CreateUser(context.Background(), user); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	jwtSvc := NewJWT("test-secret")
	token, _, err := jwtSvc.Sign(user.UserID, user.WorkspaceID, true, []string{"admins"}, "oidc")
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	handler := NewHandler(stub, jwtSvc, testAuthConfig())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	authenticated := RequireSession(jwtSvc, VerifyOptions{})
	authenticated(http.HandlerFunc(handler.me)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var response authResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.User == nil || !response.User.IsAdmin {
		t.Fatalf("expected admin user view, got %+v", response.User)
	}
}

func TestAuthConfigReturnsOIDCProvider(t *testing.T) {
	t.Parallel()

	handler := NewHandler(newStubStore(), NewJWT("test-secret"), Config{
		FrontendURL:         "http://localhost:5173",
		PasswordAuthEnabled: true,
		SignUpEnabled:       true,
		OIDC: OIDCConfig{
			Enabled:             true,
			ProviderID:          "oidc",
			ProviderName:        "Company SSO",
			IssuerURL:           "https://issuer.example.com",
			ClientID:            "client-id",
			RedirectURL:         "http://localhost:8090/api/v1/auth/oidc/callback",
			FrontendCallbackURL: "http://localhost:5173/auth/callback",
			StateSecret:         []byte("test-secret"),
		},
	})

	req := httptest.NewRequest(http.MethodGet, "http://localhost:8090/api/v1/auth/config", nil)
	rec := httptest.NewRecorder()

	handler.getAuthConfig(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var response authConfigResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Providers) != 1 {
		t.Fatalf("expected one provider, got %d", len(response.Providers))
	}
	if response.Providers[0].ProviderID != "oidc" {
		t.Fatalf("unexpected provider: %+v", response.Providers[0])
	}
	if !response.Providers[0].Enabled {
		t.Fatalf("expected provider to be enabled: %+v", response.Providers[0])
	}
}

func testAuthConfig() Config {
	return Config{
		PasswordAuthEnabled: true,
		SignUpEnabled:       true,
	}
}

type stubStore struct {
	workspacesByID   map[string]*domain.Workspace
	workspacesBySlug map[string]*domain.Workspace
	usersByID        map[string]*domain.User
	usersByEmail     map[string]*domain.User
	usersByUsername  map[string]*domain.User
	favoritesByUser  map[string]map[string]struct{}
}

func newStubStore() *stubStore {
	return &stubStore{
		workspacesByID:   map[string]*domain.Workspace{},
		workspacesBySlug: map[string]*domain.Workspace{},
		usersByID:        map[string]*domain.User{},
		usersByEmail:     map[string]*domain.User{},
		usersByUsername:  map[string]*domain.User{},
		favoritesByUser:  map[string]map[string]struct{}{},
	}
}

func (s *stubStore) CreateWorkspace(_ context.Context, workspace *domain.Workspace) error {
	if _, exists := s.workspacesByID[workspace.WorkspaceID]; exists {
		return store.ErrDuplicate
	}
	if _, exists := s.workspacesBySlug[workspace.Slug]; exists {
		return store.ErrDuplicate
	}
	s.workspacesByID[workspace.WorkspaceID] = workspace
	s.workspacesBySlug[workspace.Slug] = workspace
	return nil
}

func (s *stubStore) DeleteWorkspace(_ context.Context, id string) error {
	workspace, ok := s.workspacesByID[id]
	if !ok {
		return store.ErrNotFound
	}
	delete(s.workspacesByID, id)
	delete(s.workspacesBySlug, workspace.Slug)
	return nil
}

func (s *stubStore) GetWorkspaceByID(_ context.Context, id string) (*domain.Workspace, error) {
	workspace, ok := s.workspacesByID[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	return workspace, nil
}

func (s *stubStore) GetWorkspaceBySlug(_ context.Context, slug string) (*domain.Workspace, error) {
	workspace, ok := s.workspacesBySlug[slug]
	if !ok {
		return nil, store.ErrNotFound
	}
	return workspace, nil
}

func (s *stubStore) CreateUser(_ context.Context, user *domain.User) error {
	if _, exists := s.usersByID[user.UserID]; exists {
		return store.ErrDuplicate
	}
	if _, exists := s.usersByEmail[user.Email]; exists {
		return store.ErrDuplicate
	}
	if _, exists := s.usersByUsername[user.Username]; exists {
		return store.ErrDuplicate
	}
	s.usersByID[user.UserID] = user
	s.usersByEmail[user.Email] = user
	s.usersByUsername[user.Username] = user
	return nil
}

func (s *stubStore) GetUserByID(_ context.Context, id string) (*domain.User, error) {
	user, ok := s.usersByID[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	return user, nil
}

func (s *stubStore) GetUserByEmail(_ context.Context, email string) (*domain.User, error) {
	user, ok := s.usersByEmail[email]
	if !ok {
		return nil, store.ErrNotFound
	}
	return user, nil
}

func (s *stubStore) GetUserByUsername(_ context.Context, username string) (*domain.User, error) {
	user, ok := s.usersByUsername[username]
	if !ok {
		return nil, store.ErrNotFound
	}
	return user, nil
}

func (s *stubStore) ListFavoritePackageIDs(_ context.Context, userID string) ([]string, error) {
	packageSet, ok := s.favoritesByUser[userID]
	if !ok {
		return []string{}, nil
	}

	packageIDs := make([]string, 0, len(packageSet))
	for packageID := range packageSet {
		packageIDs = append(packageIDs, packageID)
	}
	return packageIDs, nil
}

func (s *stubStore) SaveFavoritePackage(_ context.Context, favorite *domain.FavoritePackage) error {
	if favorite == nil {
		return nil
	}
	if _, ok := s.favoritesByUser[favorite.UserID]; !ok {
		s.favoritesByUser[favorite.UserID] = map[string]struct{}{}
	}
	s.favoritesByUser[favorite.UserID][favorite.PackageID] = struct{}{}
	return nil
}

func (s *stubStore) RemoveFavoritePackage(_ context.Context, userID, packageID string) error {
	packageSet, ok := s.favoritesByUser[userID]
	if !ok {
		return nil
	}
	delete(packageSet, packageID)
	if len(packageSet) == 0 {
		delete(s.favoritesByUser, userID)
	}
	return nil
}

func (s *stubStore) Close(context.Context) error {
	return nil
}
