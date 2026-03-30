package catalog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/babelsuite/babelsuite/internal/auth"
	"github.com/babelsuite/babelsuite/internal/domain"
)

func TestListPackagesMarksUserFavorites(t *testing.T) {
	t.Parallel()

	service := stubCatalogService{
		packages: []Package{
			{ID: "payment-suite", Title: "Payment Suite"},
			{ID: "fleet-control-room", Title: "Fleet Control Room"},
		},
	}
	favorites := newStubFavoriteStore()
	if err := favorites.SaveFavoritePackage(context.Background(), &domain.FavoritePackage{
		UserID:    "user-1",
		PackageID: "payment-suite",
	}); err != nil {
		t.Fatalf("seed favorite: %v", err)
	}

	handler := NewHandler(service, favorites, auth.NewJWT("test-secret"))
	mux := http.NewServeMux()
	handler.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/packages", nil)
	req.Header.Set("Authorization", "Bearer "+mustSignCatalogToken(t, "user-1"))
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var response struct {
		Packages []Package `json:"packages"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Packages) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(response.Packages))
	}
	if !response.Packages[0].Starred {
		t.Fatal("expected payment-suite to be starred for this user")
	}
	if response.Packages[1].Starred {
		t.Fatal("expected other package to remain unstarred")
	}
}

func TestFavoriteEndpointsPersistAndRemoveStars(t *testing.T) {
	t.Parallel()

	service := stubCatalogService{
		packages: []Package{{ID: "payment-suite", Title: "Payment Suite"}},
	}
	favorites := newStubFavoriteStore()
	handler := NewHandler(service, favorites, auth.NewJWT("test-secret"))
	mux := http.NewServeMux()
	handler.Register(mux)
	token := "Bearer " + mustSignCatalogToken(t, "user-1")

	addReq := httptest.NewRequest(http.MethodPost, "/api/v1/catalog/favorites/payment-suite", nil)
	addReq.Header.Set("Authorization", token)
	addRec := httptest.NewRecorder()
	mux.ServeHTTP(addRec, addReq)

	if addRec.Code != http.StatusOK {
		t.Fatalf("expected add status %d, got %d", http.StatusOK, addRec.Code)
	}

	packageIDs, err := favorites.ListFavoritePackageIDs(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("list favorites after add: %v", err)
	}
	if len(packageIDs) != 1 || packageIDs[0] != "payment-suite" {
		t.Fatalf("expected saved favorite, got %#v", packageIDs)
	}

	removeReq := httptest.NewRequest(http.MethodDelete, "/api/v1/catalog/favorites/payment-suite", nil)
	removeReq.Header.Set("Authorization", token)
	removeRec := httptest.NewRecorder()
	mux.ServeHTTP(removeRec, removeReq)

	if removeRec.Code != http.StatusOK {
		t.Fatalf("expected remove status %d, got %d", http.StatusOK, removeRec.Code)
	}

	packageIDs, err = favorites.ListFavoritePackageIDs(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("list favorites after remove: %v", err)
	}
	if len(packageIDs) != 0 {
		t.Fatalf("expected favorites to be removed, got %#v", packageIDs)
	}
}

func mustSignCatalogToken(t *testing.T, userID string) string {
	t.Helper()

	jwt := auth.NewJWT("test-secret")
	token, _, err := jwt.Sign(userID, "workspace-1", false)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return token
}

type stubCatalogService struct {
	packages []Package
}

func (s stubCatalogService) ListPackages() ([]Package, error) {
	return clonePackages(s.packages), nil
}

func (s stubCatalogService) GetPackage(id string) (*Package, error) {
	for _, item := range s.packages {
		if item.ID == id {
			clone := item
			return &clone, nil
		}
	}
	return nil, ErrNotFound
}

type stubFavoriteStore struct {
	favorites map[string]map[string]struct{}
}

func newStubFavoriteStore() *stubFavoriteStore {
	return &stubFavoriteStore{
		favorites: map[string]map[string]struct{}{},
	}
}

func (s *stubFavoriteStore) ListFavoritePackageIDs(_ context.Context, userID string) ([]string, error) {
	packageSet, ok := s.favorites[userID]
	if !ok {
		return []string{}, nil
	}
	packageIDs := make([]string, 0, len(packageSet))
	for packageID := range packageSet {
		packageIDs = append(packageIDs, packageID)
	}
	return packageIDs, nil
}

func (s *stubFavoriteStore) SaveFavoritePackage(_ context.Context, favorite *domain.FavoritePackage) error {
	if favorite == nil {
		return nil
	}
	if _, ok := s.favorites[favorite.UserID]; !ok {
		s.favorites[favorite.UserID] = map[string]struct{}{}
	}
	s.favorites[favorite.UserID][favorite.PackageID] = struct{}{}
	return nil
}

func (s *stubFavoriteStore) RemoveFavoritePackage(_ context.Context, userID, packageID string) error {
	packageSet, ok := s.favorites[userID]
	if !ok {
		return nil
	}
	delete(packageSet, packageID)
	if len(packageSet) == 0 {
		delete(s.favorites, userID)
	}
	return nil
}

var _ favoriteStore = (*stubFavoriteStore)(nil)
