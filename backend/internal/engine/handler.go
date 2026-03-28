package engine

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/babelsuite/babelsuite/internal/auth"
)

type Handler struct {
	store *Store
	jwt   *auth.JWTService
}

func NewHandler(store *Store, jwt *auth.JWTService) *Handler {
	return &Handler{store: store, jwt: jwt}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/engine/overview", h.getOverview)
	mux.HandleFunc("GET /api/v1/engine/overview/stream", h.streamOverview)
}

func (h *Handler) getOverview(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}

	writeJSON(w, http.StatusOK, h.store.Overview())
}

func (h *Handler) streamOverview(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "Streaming is not supported.")
		return
	}

	stream := h.store.Subscribe(r.Context())
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
		case overview := <-stream:
			payload, err := json.Marshal(overview)
			if err != nil {
				continue
			}
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
