package engine

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/babelsuite/babelsuite/internal/auth"
	"github.com/babelsuite/babelsuite/internal/httpserver"
)

type Handler struct {
	store *Store
	jwt   *auth.JWTService
}

func NewHandler(store *Store, jwt *auth.JWTService) *Handler {
	return &Handler{store: store, jwt: jwt}
}

func (h *Handler) Register(mux *http.ServeMux) {
	protected := auth.RequireSession(h.jwt, auth.VerifyOptions{})
	streaming := auth.RequireSession(h.jwt, auth.VerifyOptions{AllowQueryToken: true})
	httpserver.HandleFunc(mux, "GET /api/v1/engine/overview", h.getOverview, protected)
	httpserver.HandleFunc(mux, "GET /api/v1/engine/overview/stream", h.streamOverview, streaming)
}

func (h *Handler) getOverview(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.store.Overview())
}

func (h *Handler) streamOverview(w http.ResponseWriter, r *http.Request) {
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
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
