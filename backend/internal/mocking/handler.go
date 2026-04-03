package mocking

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/babelsuite/babelsuite/internal/suites"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/internal/mock-data/", h.resolveOperation)
	mux.HandleFunc("/mocks/rest/", h.invokeREST)
	mux.HandleFunc("POST /mocks/grpc/{suiteId}/{surfaceId}/{operationId}", h.invokeGRPC)
	mux.HandleFunc("POST /mocks/async/{suiteId}/{surfaceId}/{operationId}", h.invokeAsync)
}

func (h *Handler) resolveOperation(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/internal/mock-data/")
	parts := strings.SplitN(strings.Trim(trimmed, "/"), "/", 3)
	if len(parts) < 3 {
		writeError(w, http.StatusNotFound, "Mock resolver route not found.")
		return
	}

	result, err := h.service.ResolveOperation(r.Context(), parts[0], parts[1], parts[2], r)
	if err != nil {
		h.writeLookupError(w, err)
		return
	}
	writeResolverEnvelope(w, result)
}

func (h *Handler) invokeREST(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/mocks/rest/")
	parts := strings.SplitN(strings.Trim(trimmed, "/"), "/", 3)
	if len(parts) < 3 {
		writeError(w, http.StatusNotFound, "Mock route not found.")
		return
	}

	result, err := h.service.InvokeREST(r.Context(), parts[0], parts[1], "/"+parts[2], r)
	if err != nil {
		h.writeLookupError(w, err)
		return
	}
	writeResult(w, result)
}

func (h *Handler) invokeGRPC(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.InvokeAdapter(r.Context(), r.PathValue("suiteId"), r.PathValue("surfaceId"), r.PathValue("operationId"), "grpc", r)
	if err != nil {
		h.writeLookupError(w, err)
		return
	}
	writeResult(w, result)
}

func (h *Handler) invokeAsync(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.InvokeAdapter(r.Context(), r.PathValue("suiteId"), r.PathValue("surfaceId"), r.PathValue("operationId"), "async", r)
	if err != nil {
		h.writeLookupError(w, err)
		return
	}
	writeResult(w, result)
}

func (h *Handler) writeLookupError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, suites.ErrNotFound):
		writeError(w, http.StatusNotFound, "Suite not found.")
	case errors.Is(err, ErrSurfaceNotFound):
		writeError(w, http.StatusNotFound, "Mock surface not found.")
	case errors.Is(err, ErrOperationNotFound):
		writeError(w, http.StatusNotFound, "Mock operation not found.")
	default:
		writeError(w, http.StatusInternalServerError, "Could not process mock invocation.")
	}
}

func writeResult(w http.ResponseWriter, result *Result) {
	if result == nil {
		writeError(w, http.StatusInternalServerError, "Mock result was empty.")
		return
	}

	for key, values := range result.Headers {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	if strings.TrimSpace(result.MediaType) != "" {
		w.Header().Set("Content-Type", result.MediaType)
	}
	if strings.TrimSpace(result.RuntimeURL) != "" {
		w.Header().Set("X-Babelsuite-Runtime-Url", result.RuntimeURL)
	}
	if strings.TrimSpace(result.Adapter) != "" {
		w.Header().Set("X-Babelsuite-Mock-Adapter", result.Adapter)
	}
	if strings.TrimSpace(result.Dispatcher) != "" {
		w.Header().Set("X-Babelsuite-Dispatcher", result.Dispatcher)
	}
	if strings.TrimSpace(result.ResolverURL) != "" {
		w.Header().Set("X-Babelsuite-Resolver-Url", result.ResolverURL)
	}
	if strings.TrimSpace(result.MatchedExample) != "" {
		w.Header().Set("X-Babelsuite-Mock-Example", result.MatchedExample)
	}
	w.WriteHeader(result.Status)
	_, _ = w.Write(result.Body)
}

func writeResolverEnvelope(w http.ResponseWriter, result *Result) {
	if result == nil {
		writeError(w, http.StatusInternalServerError, "Mock result was empty.")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	envelope := map[string]any{
		"status":         result.Status,
		"mediaType":      result.MediaType,
		"headers":        result.Headers,
		"body":           string(result.Body),
		"adapter":        result.Adapter,
		"dispatcher":     result.Dispatcher,
		"resolverUrl":    result.ResolverURL,
		"runtimeUrl":     result.RuntimeURL,
		"matchedExample": result.MatchedExample,
	}
	_ = json.NewEncoder(w).Encode(envelope)
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
