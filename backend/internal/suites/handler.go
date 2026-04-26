package suites

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/babelsuite/babelsuite/internal/auth"
	"github.com/babelsuite/babelsuite/internal/httpserver"
)


type Handler struct {
	service Reader
	jwt     *auth.JWTService
}

type Reader interface {
	List() []Definition
	Get(id string) (*Definition, error)
	Register(req RegisterRequest) (Definition, error)
}

func NewHandler(service Reader, jwt *auth.JWTService) *Handler {
	return &Handler{service: service, jwt: jwt}
}

func (h *Handler) Register(mux *http.ServeMux) {
	protected := auth.RequireSession(h.jwt, auth.VerifyOptions{})
	httpserver.HandleFunc(mux, "GET /api/v1/suites", h.listSuites, protected)
	httpserver.HandleFunc(mux, "GET /api/v1/suites/{suiteId}", h.getSuite, protected)
	httpserver.HandleFunc(mux, "POST /api/v1/suites", h.createSuite, protected)
}

func (h *Handler) listSuites(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"suites": h.service.List()})
}

func (h *Handler) createSuite(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Suite payload is invalid.")
		return
	}
	definition, err := h.service.Register(req)
	if err != nil {
		switch {
		case errors.Is(err, ErrAlreadyExists):
			writeError(w, http.StatusConflict, "A suite with this ID already exists.")
		default:
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusCreated, definition)
}

func (h *Handler) getSuite(w http.ResponseWriter, r *http.Request) {
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
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
