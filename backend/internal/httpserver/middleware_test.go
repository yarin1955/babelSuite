package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestIDMiddlewareSetsResponseHeader(t *testing.T) {
	handler := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if RequestIDFromContext(r.Context()) == "" {
			t.Fatal("expected request id in context")
		}
		w.WriteHeader(http.StatusNoContent)
	}), RequestIDMiddleware())

	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Header().Get(RequestIDHeader) == "" {
		t.Fatal("expected request id response header")
	}
}

func TestHandleStoresRoutePatternInContext(t *testing.T) {
	mux := http.NewServeMux()
	HandleFunc(mux, "GET /api/v1/example", func(w http.ResponseWriter, r *http.Request) {
		if got := RoutePatternFromContext(r.Context()); got != "GET /api/v1/example" {
			t.Fatalf("expected route pattern, got %q", got)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/example", nil)
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", response.Code)
	}
}
