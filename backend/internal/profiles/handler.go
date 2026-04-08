package profiles

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/babelsuite/babelsuite/internal/auth"
	"github.com/babelsuite/babelsuite/internal/httpserver"
)

type Handler struct {
	service *Service
	jwt     *auth.JWTService
}

func NewHandler(service *Service, jwt *auth.JWTService) *Handler {
	return &Handler{service: service, jwt: jwt}
}

func (h *Handler) Register(mux *http.ServeMux) {
	protected := auth.RequireSession(h.jwt, auth.VerifyOptions{})
	httpserver.HandleFunc(mux, "GET /api/v1/profiles/suites", h.listSuites, protected)
	httpserver.HandleFunc(mux, "GET /api/v1/profiles/suites/{suiteId}", h.getSuiteProfiles, protected)
	httpserver.HandleFunc(mux, "POST /api/v1/profiles/suites/{suiteId}", h.createProfile, protected)
	httpserver.HandleFunc(mux, "PUT /api/v1/profiles/suites/{suiteId}/{profileId}", h.updateProfile, protected)
	httpserver.HandleFunc(mux, "DELETE /api/v1/profiles/suites/{suiteId}/{profileId}", h.deleteProfile, protected)
	httpserver.HandleFunc(mux, "POST /api/v1/profiles/suites/{suiteId}/{profileId}/default", h.setDefaultProfile, protected)
}

func (h *Handler) listSuites(w http.ResponseWriter, r *http.Request) {
	summaries, err := h.service.ListSuiteSummaries()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load profile suites.")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"suites": summaries})
}

func (h *Handler) getSuiteProfiles(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.service.GetSuiteProfiles(r.PathValue("suiteId"))
	if err != nil {
		h.writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, profiles)
}

func (h *Handler) createProfile(w http.ResponseWriter, r *http.Request) {
	var request UpsertRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid profile payload.")
		return
	}

	profiles, err := h.service.CreateProfile(r.PathValue("suiteId"), request)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, profiles)
}

func (h *Handler) updateProfile(w http.ResponseWriter, r *http.Request) {
	var request UpsertRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid profile payload.")
		return
	}

	profiles, err := h.service.UpdateProfile(r.PathValue("suiteId"), r.PathValue("profileId"), request)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, profiles)
}

func (h *Handler) deleteProfile(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.service.DeleteProfile(r.PathValue("suiteId"), r.PathValue("profileId"))
	if err != nil {
		h.writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, profiles)
}

func (h *Handler) setDefaultProfile(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.service.SetDefaultProfile(r.PathValue("suiteId"), r.PathValue("profileId"))
	if err != nil {
		h.writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, profiles)
}

func (h *Handler) writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrSuiteNotFound):
		writeError(w, http.StatusNotFound, "Suite not found.")
	case errors.Is(err, ErrProfileNotFound):
		writeError(w, http.StatusNotFound, "Profile not found.")
	default:
		writeError(w, http.StatusBadRequest, err.Error())
	}
}
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
