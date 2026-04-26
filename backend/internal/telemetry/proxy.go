package telemetry

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func NewTraceProxyHandler() http.Handler {
	return newOTLPProxyHandler("/v1/traces")
}

func NewMetricsProxyHandler() http.Handler {
	return newOTLPProxyHandler("/v1/metrics")
}

func newOTLPProxyHandler(collectorPath string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
		if endpoint == "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		target, schemeHint := normalizeCollectorEndpoint(endpoint)
		scheme := "https"
		if shouldSkipTLS(target, schemeHint) {
			scheme = "http"
		}
		forwardURL := fmt.Sprintf("%s://%s%s", scheme, strings.TrimRight(target, "/"), collectorPath)

		req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, forwardURL, r.Body)
		if err != nil {
			http.Error(w, "failed to build upstream request", http.StatusBadGateway)
			return
		}

		if ct := r.Header.Get("Content-Type"); ct != "" {
			req.Header.Set("Content-Type", ct)
		}
		for key, value := range readCollectorHeaders() {
			req.Header.Set(key, value)
		}

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			http.Error(w, "upstream collector unreachable", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	})
}
