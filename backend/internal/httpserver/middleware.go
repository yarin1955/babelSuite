package httpserver

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/babelsuite/babelsuite/internal/auth"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

type Middleware func(http.Handler) http.Handler

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}


type HTTPMetrics struct {
	requests  metric.Int64Counter
	active    metric.Int64UpDownCounter
	durations metric.Float64Histogram
}

func Chain(next http.Handler, middleware ...Middleware) http.Handler {
	if next == nil {
		next = http.NotFoundHandler()
	}
	for index := len(middleware) - 1; index >= 0; index-- {
		next = middleware[index](next)
	}
	return next
}

func Handle(mux *http.ServeMux, pattern string, handler http.Handler, middleware ...Middleware) {
	if mux == nil {
		return
	}
	chain := append([]Middleware{routePatternMiddleware(pattern)}, middleware...)
	mux.Handle(pattern, Chain(handler, chain...))
}

func HandleFunc(mux *http.ServeMux, pattern string, handler func(http.ResponseWriter, *http.Request), middleware ...Middleware) {
	Handle(mux, pattern, http.HandlerFunc(handler), middleware...)
}

func NewHTTPMetrics() *HTTPMetrics {
	meter := otel.Meter("github.com/babelsuite/babelsuite/internal/httpserver")
	requests, _ := meter.Int64Counter("http.server.requests")
	active, _ := meter.Int64UpDownCounter("http.server.active_requests")
	durations, _ := meter.Float64Histogram("http.server.duration.ms")
	return &HTTPMetrics{
		requests:  requests,
		active:    active,
		durations: durations,
	}
}

func (m *HTTPMetrics) Middleware() Middleware {
	return func(next http.Handler) http.Handler {
		if next == nil {
			next = http.NotFoundHandler()
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			startedAt := time.Now()
			attrs := requestAttributes(r, http.StatusOK)

			if m != nil {
				m.active.Add(r.Context(), 1, metric.WithAttributes(attrs...))
				defer m.active.Add(r.Context(), -1, metric.WithAttributes(attrs...))
			}

			next.ServeHTTP(recorder, r)

			attrs = requestAttributes(r, recorder.status)
			if m != nil {
				m.requests.Add(r.Context(), 1, metric.WithAttributes(attrs...))
				m.durations.Record(r.Context(), float64(time.Since(startedAt).Milliseconds()), metric.WithAttributes(attrs...))
			}
		})
	}
}

func TraceContextMiddleware() Middleware {
	return func(next http.Handler) http.Handler {
		if next == nil {
			next = http.NotFoundHandler()
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			span := trace.SpanFromContext(r.Context())
			if span.IsRecording() {
				if requestID := RequestIDFromContext(r.Context()); requestID != "" {
					span.SetAttributes(attribute.String("http.request_id", requestID))
				}
				if claims, ok := auth.SessionFromContext(r.Context()); ok {
					span.SetAttributes(
						attribute.String("enduser.id", claims.UserID),
						attribute.String("enduser.workspace_id", claims.WorkspaceID),
						attribute.Bool("enduser.admin", claims.IsAdmin),
					)
				}
				// Defer route annotation until after the mux has matched the pattern
				// so span names are "METHOD /path/{param}" rather than raw URLs.
				defer func() {
					if pattern := effectiveRoute(r); pattern != "" {
						span.SetName(r.Method + " " + pattern)
						span.SetAttributes(attribute.String("http.route", pattern))
					}
				}()
			}
			next.ServeHTTP(w, r)
		})
	}
}

func AuditMiddleware() Middleware {
	return func(next http.Handler) http.Handler {
		if next == nil {
			next = http.NotFoundHandler()
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			startedAt := time.Now()
			next.ServeHTTP(recorder, r)

			if !shouldAuditRequest(r) {
				return
			}

			attrs := []slog.Attr{
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", recorder.status),
				slog.Int64("durationMs", time.Since(startedAt).Milliseconds()),
				slog.String("remoteAddr", strings.TrimSpace(r.RemoteAddr)),
			}
			if id := RequestIDFromContext(r.Context()); id != "" {
				attrs = append(attrs, slog.String("requestId", id))
			}
			if route := effectiveRoute(r); route != "" {
				attrs = append(attrs, slog.String("route", route))
			}
			if claims, ok := auth.SessionFromContext(r.Context()); ok {
				attrs = append(attrs, slog.String("userId", claims.UserID))
				attrs = append(attrs, slog.String("workspaceId", claims.WorkspaceID))
			}
			slog.LogAttrs(r.Context(), slog.LevelInfo, "http.request", attrs...)
		})
	}
}

func (w *statusRecorder) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusRecorder) Write(payload []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	written, err := w.ResponseWriter.Write(payload)
	w.bytes += written
	return written, err
}

func (w *statusRecorder) Flush() {
	flusher, ok := w.ResponseWriter.(http.Flusher)
	if !ok {
		return
	}
	flusher.Flush()
}

func requestAttributes(r *http.Request, status int) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("http.method", r.Method),
		attribute.String("http.route", effectiveRoute(r)),
		attribute.String("http.status_code", strconv.Itoa(status)),
	}
}

func effectiveRoute(r *http.Request) string {
	if pattern := strings.TrimSpace(RoutePatternFromContext(r.Context())); pattern != "" {
		return pattern
	}
	if pattern := strings.TrimSpace(r.Pattern); pattern != "" {
		return pattern
	}
	path := strings.TrimSpace(r.URL.Path)
	if path == "" {
		return "/"
	}
	return path
}

const contentSecurityPolicy = "default-src 'self'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: blob:; " +
	"connect-src 'self' ws: wss:; " +
	"font-src 'self'; " +
	"object-src 'none'; " +
	"base-uri 'self'; " +
	"form-action 'self'"

func RecoveryMiddleware() Middleware {
	return func(next http.Handler) http.Handler {
		if next == nil {
			next = http.NotFoundHandler()
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					slog.ErrorContext(r.Context(), "handler panic",
						slog.Any("error", rec),
						slog.String("stack", string(debug.Stack())),
					)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"error":"An unexpected error occurred."}`))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func SecurityHeadersMiddleware() Middleware {
	return func(next http.Handler) http.Handler {
		if next == nil {
			next = http.NotFoundHandler()
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			w.Header().Set("X-Permitted-Cross-Domain-Policies", "none")
			w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
			if r.TLS != nil || strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https") {
				w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}

func BodyLimitMiddleware(limit int64) Middleware {
	return func(next http.Handler) http.Handler {
		if next == nil {
			next = http.NotFoundHandler()
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > limit {
				http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
				return
			}
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, limit)
			}
			next.ServeHTTP(w, r)
		})
	}
}

func shouldAuditRequest(r *http.Request) bool {
	if r.Method == http.MethodOptions {
		return false
	}
	path := strings.TrimSpace(r.URL.Path)
	if path == "/healthz" || path == "/readyz" || strings.HasPrefix(path, "/readyz/") {
		return false
	}
	return strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/auth/")
}
