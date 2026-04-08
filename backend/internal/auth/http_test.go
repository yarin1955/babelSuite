package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireSessionUsesBearerToken(t *testing.T) {
	jwt := NewJWT("test-secret")
	token, _, err := jwt.Sign("user-1", "workspace-1", true, []string{"admins"}, "password")
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	handler := RequireSession(jwt, VerifyOptions{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := SessionFromContext(r.Context())
		if !ok {
			t.Fatal("expected claims in context")
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"userId": claims.UserID})
	}))

	request := httptest.NewRequest(http.MethodGet, "/api/v1/private", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
}

func TestPopulateSessionIgnoresInvalidToken(t *testing.T) {
	jwt := NewJWT("test-secret")

	handler := PopulateSession(jwt, VerifyOptions{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := SessionFromContext(r.Context()); ok {
			t.Fatal("did not expect claims in context")
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodGet, "/api/v1/public", nil)
	request.Header.Set("Authorization", "Bearer invalid")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", response.Code)
	}
}

func TestRequireSessionAllowsQueryTokenWhenEnabled(t *testing.T) {
	jwt := NewJWT("test-secret")
	token, _, err := jwt.Sign("user-1", "workspace-1", false, nil, "password")
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	handler := RequireSession(jwt, VerifyOptions{AllowQueryToken: true})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodGet, "/api/v1/stream?token="+token, nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", response.Code)
	}
}
