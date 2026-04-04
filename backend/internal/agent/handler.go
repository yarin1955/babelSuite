package agent

import (
	"bufio"
	"encoding/json"
	"net/http"
)

func NewHandler(service *Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/v1/agent/info", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, service.Info())
	})
	mux.HandleFunc("POST /api/v1/agent/run", func(w http.ResponseWriter, r *http.Request) {
		var request StepRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request payload is invalid"})
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming is not supported"})
			return
		}

		w.Header().Set("Content-Type", "application/x-ndjson")
		writer := bufio.NewWriter(w)
		service.Stream(r.Context(), request, func(message StreamMessage) {
			_ = json.NewEncoder(writer).Encode(message)
			_ = writer.Flush()
			flusher.Flush()
		})
	})
	mux.HandleFunc("POST /api/v1/agent/jobs/{jobId}/cancel", func(w http.ResponseWriter, r *http.Request) {
		if service.Cancel(r.PathValue("jobId")) {
			writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
			return
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
	})
	mux.HandleFunc("POST /api/v1/agent/jobs/{jobId}/cleanup", func(w http.ResponseWriter, r *http.Request) {
		if service.Cleanup(r.PathValue("jobId")) {
			writeJSON(w, http.StatusOK, map[string]string{"status": "cleaned"})
			return
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
	})
	return mux
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
