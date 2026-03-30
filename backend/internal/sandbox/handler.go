package sandbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/babelsuite/babelsuite/internal/auth"
)

type Handler struct {
	manager Manager
	jwt     *auth.JWTService
}

func NewHandler(manager Manager, jwt *auth.JWTService) *Handler {
	return &Handler{manager: manager, jwt: jwt}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/sandboxes", h.listSandboxes)
	mux.HandleFunc("GET /api/v1/sandboxes/events", h.streamEvents)
	mux.HandleFunc("POST /api/v1/sandboxes/reap-all", h.reapAll)
	mux.HandleFunc("POST /api/v1/sandboxes/{sandboxId}/reap", h.reapSandbox)
}

func (h *Handler) listSandboxes(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}

	inventory, err := h.manager.Snapshot(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load sandboxes.")
		return
	}

	writeJSON(w, http.StatusOK, inventory)
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

	stream, err := h.manager.SubscribeEvents(r.Context(), since)
	if err != nil {
		if errors.Is(err, ErrDockerUnavailable) {
			writeError(w, http.StatusServiceUnavailable, humanizeDockerError(err))
			return
		}
		writeError(w, http.StatusInternalServerError, "Could not stream sandbox events.")
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
		case event, ok := <-stream:
			if !ok {
				return
			}
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

func (h *Handler) reapSandbox(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}

	result, err := h.manager.ReapSandbox(r.Context(), r.PathValue("sandboxId"))
	if err != nil {
		h.writeManagerError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) reapAll(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}

	result, err := h.manager.ReapAll(r.Context())
	if err != nil {
		h.writeManagerError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) writeManagerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "Sandbox not found.")
	case errors.Is(err, ErrDockerUnavailable):
		writeError(w, http.StatusServiceUnavailable, humanizeDockerError(err))
	default:
		writeError(w, http.StatusInternalServerError, "Sandbox cleanup failed.")
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
