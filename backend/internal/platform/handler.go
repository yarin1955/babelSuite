package platform

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/babelsuite/babelsuite/internal/auth"
	"github.com/babelsuite/babelsuite/internal/httpserver"
)

type Handler struct {
	store Store
	jwt   *auth.JWTService
}

func NewHandler(store Store, jwt *auth.JWTService) *Handler {
	return &Handler{store: store, jwt: jwt}
}

func (h *Handler) Register(mux *http.ServeMux) {
	protected := auth.RequireSession(h.jwt, auth.VerifyOptions{})
	admin := auth.RequireAdmin(h.jwt)
	httpserver.HandleFunc(mux, "GET /api/v1/platform-settings", h.getSettings, protected)
	httpserver.HandleFunc(mux, "PUT /api/v1/platform-settings", h.updateSettings, admin)
	httpserver.HandleFunc(mux, "POST /api/v1/platform-settings/registries/{registryId}/sync", h.syncRegistry, admin)
}

func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.store.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load platform settings.")
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (h *Handler) updateSettings(w http.ResponseWriter, r *http.Request) {
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
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
