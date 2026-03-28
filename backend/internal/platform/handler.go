package platform

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/babelsuite/babelsuite/internal/auth"
)

type Handler struct {
	store Store
	jwt   *auth.JWTService
}

func NewHandler(store Store, jwt *auth.JWTService) *Handler {
	return &Handler{store: store, jwt: jwt}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/platform-settings", h.getSettings)
	mux.HandleFunc("PUT /api/v1/platform-settings", h.updateSettings)
	mux.HandleFunc("POST /api/v1/platform-settings/registries/{registryId}/sync", h.syncRegistry)
}

func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}

	settings, err := h.store.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load platform settings.")
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (h *Handler) updateSettings(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}

	var settings PlatformSettings
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid platform settings payload.")
		return
	}

	normalize(&settings)
	if err := validate(&settings); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.store.Save(&settings); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not save platform settings.")
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (h *Handler) syncRegistry(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}

	registryID := strings.TrimSpace(r.PathValue("registryId"))
	if registryID == "" {
		writeError(w, http.StatusBadRequest, "Registry ID is required.")
		return
	}

	settings, err := h.store.SyncRegistry(registryID)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			writeError(w, http.StatusNotFound, "Registry not found.")
		default:
			writeError(w, http.StatusInternalServerError, "Could not sync registry.")
		}
		return
	}

	writeJSON(w, http.StatusOK, settings)
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

