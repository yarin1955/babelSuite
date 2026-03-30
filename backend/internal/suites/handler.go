package suites

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/babelsuite/babelsuite/internal/auth"
)

type Handler struct {
	service Reader
	jwt     *auth.JWTService
}

type Reader interface {
	List() []Definition
	Get(id string) (*Definition, error)
}

func NewHandler(service Reader, jwt *auth.JWTService) *Handler {
	return &Handler{service: service, jwt: jwt}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/suites", h.listSuites)
	mux.HandleFunc("GET /api/v1/suites/{suiteId}", h.getSuite)
}

func (h *Handler) listSuites(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"suites": h.service.List()})
}

func (h *Handler) getSuite(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}

	suite, err := h.service.Get(r.PathValue("suiteId"))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "Suite not found.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Could not load suite.")
		return
	}

	writeJSON(w, http.StatusOK, suite)
}

func (h *Handler) requireAuth(w http.ResponseWriter, r *http.Request) bool {
	bearer := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(bearer, "Bearer ") {
		writeError(w, http.StatusUnauthorized, "Sign in required.")
		return false
	}

	token := strings.TrimSpace(strings.TrimPrefix(bearer, "Bearer "))
	if token == "" {
		writeError(w, http.StatusUnauthorized, "Sign in required.")
		return false
	}

	if _, err := h.jwt.Verify(token); err != nil {
		writeError(w, http.StatusUnauthorized, "Session expired or invalid.")
		return false
	}

	return true
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
