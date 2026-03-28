package execution

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/babelsuite/babelsuite/internal/auth"
	"github.com/babelsuite/babelsuite/internal/engine"
)

type Handler struct {
	service *Service
	engine  *engine.Store
	jwt     *auth.JWTService
}

func NewHandler(service *Service, engineStore *engine.Store, jwt *auth.JWTService) *Handler {
	return &Handler{service: service, engine: engineStore, jwt: jwt}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/executions/launch-suites", h.listLaunchSuites)
	mux.HandleFunc("GET /api/v1/executions/overview", h.getOverview)
	mux.HandleFunc("GET /api/v1/executions", h.listExecutions)
	mux.HandleFunc("POST /api/v1/executions", h.createExecution)
	mux.HandleFunc("GET /api/v1/executions/{executionId}", h.getExecution)
	mux.HandleFunc("GET /api/v1/executions/{executionId}/events", h.streamEvents)
	mux.HandleFunc("GET /api/v1/executions/{executionId}/logs", h.streamLogs)
}

func (h *Handler) listLaunchSuites(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"suites": h.service.ListLaunchSuites()})
}

func (h *Handler) getOverview(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}

	if h.engine == nil {
		writeJSON(w, http.StatusOK, engine.Overview{})
		return
	}

	writeJSON(w, http.StatusOK, h.engine.Overview())
}

func (h *Handler) listExecutions(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"executions": h.service.ListExecutions()})
}

func (h *Handler) createExecution(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}

	var request CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "Execution payload is invalid.")
		return
	}

	execution, err := h.service.CreateExecution(r.Context(), request)
	if err != nil {
		switch {
		case errors.Is(err, ErrSuiteNotFound):
			writeError(w, http.StatusNotFound, "Suite not found.")
		case errors.Is(err, ErrProfileNotFound):
			writeError(w, http.StatusBadRequest, "Selected profile does not belong to this suite.")
		default:
			writeError(w, http.StatusInternalServerError, "Could not create execution.")
		}
		return
	}

	writeJSON(w, http.StatusCreated, execution)
}

func (h *Handler) getExecution(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}

	execution, err := h.service.GetExecution(r.PathValue("executionId"))
	if err != nil {
		if errors.Is(err, ErrExecutionNotFound) {
			writeError(w, http.StatusNotFound, "Execution not found.")
			return
		}

		writeError(w, http.StatusInternalServerError, "Could not load execution.")
		return
	}

	writeJSON(w, http.StatusOK, execution)
}

func (h *Handler) streamEvents(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "Streaming is not supported.")
		return
	}

	since := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("since")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 0 {
			since = parsed
		}
	}
	if raw := strings.TrimSpace(r.Header.Get("Last-Event-ID")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > since {
			since = parsed
		}
	}

	stream, err := h.service.SubscribeEvents(r.Context(), r.PathValue("executionId"), since)
	if err != nil {
		if errors.Is(err, ErrExecutionNotFound) {
			writeError(w, http.StatusNotFound, "Execution not found.")
			return
		}

		writeError(w, http.StatusInternalServerError, "Could not stream execution events.")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	_, _ = fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			_, _ = fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case event := <-stream:
			payload, err := json.Marshal(event)
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintf(w, "id: %d\n", event.ID)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		}
	}
}

func (h *Handler) streamLogs(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "Streaming is not supported.")
		return
	}

	since := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("since")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 0 {
			since = parsed
		}
	}
	if raw := strings.TrimSpace(r.Header.Get("Last-Event-ID")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > since {
			since = parsed
		}
	}

	stream, err := h.service.SubscribeLogs(r.Context(), r.PathValue("executionId"), since)
	if err != nil {
		if errors.Is(err, ErrExecutionNotFound) {
			writeError(w, http.StatusNotFound, "Execution not found.")
			return
		}

		writeError(w, http.StatusInternalServerError, "Could not stream execution logs.")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	_, _ = fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			_, _ = fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case record := <-stream:
			payload, err := json.Marshal(record)
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintf(w, "id: %d\n", record.ID)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		}
	}
}

func (h *Handler) requireAuth(w http.ResponseWriter, r *http.Request) bool {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		bearer := strings.TrimSpace(r.Header.Get("Authorization"))
		if strings.HasPrefix(bearer, "Bearer ") {
			token = strings.TrimSpace(strings.TrimPrefix(bearer, "Bearer "))
		}
	}

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
