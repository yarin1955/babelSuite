package catalog

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/babelsuite/babelsuite/internal/auth"
)

type Handler struct {
	service *Service
	jwt     *auth.JWTService
}

func NewHandler(service *Service, jwt *auth.JWTService) *Handler {
	return &Handler{service: service, jwt: jwt}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/catalog/packages", h.listPackages)
	mux.HandleFunc("GET /api/v1/catalog/packages/{packageId}", h.getPackage)
}

func (h *Handler) listPackages(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"packages": h.service.ListPackages()})
}

func (h *Handler) getPackage(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}

	item, err := h.service.GetPackage(r.PathValue("packageId"))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "Catalog package not found.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Could not load catalog package.")
		return
	}

	writeJSON(w, http.StatusOK, item)
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
