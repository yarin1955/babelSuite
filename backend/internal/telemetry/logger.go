package telemetry

import (
	"context"
	"log"
	"log/slog"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
)

const loggerScope = "github.com/babelsuite/babelsuite"

// NewLogger returns a slog.Logger that emits records to the OTel log pipeline
// when telemetry is configured, or to JSON stdout otherwise.
func NewLogger(p *Pipeline) *slog.Logger {
	level := parseLogLevel(strings.TrimSpace(os.Getenv("LOG_LEVEL")))
	if p != nil && p.logProvider != nil {
		handler := otelslog.NewHandler(loggerScope,
			otelslog.WithLoggerProvider(p.logProvider),
		)
		return slog.New(levelHandler{min: level, next: handler})
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}

// InitDefaultLogger installs logger as the process-wide default for slog and
// redirects the standard library log package through it at Info level, so that
// existing log.Printf calls are captured without changing every call site.
func InitDefaultLogger(logger *slog.Logger) {
	slog.SetDefault(logger)
	log.SetFlags(0)
	log.SetOutput(&stdLogWriter{handler: logger.Handler()})
}

// stdLogWriter satisfies io.Writer and forwards each line written by the
// standard log package into slog at Info level.
type stdLogWriter struct{ handler slog.Handler }

func (w *stdLogWriter) Write(p []byte) (int, error) {
	msg := strings.TrimRight(string(p), "\r\n")
	if msg == "" {
		return len(p), nil
	}
	r := slog.NewRecord(time.Now(), slog.LevelInfo, msg, 0)
	_ = w.handler.Handle(context.Background(), r)
	return len(p), nil
}

// levelHandler is a minimal slog.Handler wrapper that enforces a minimum level.
type levelHandler struct {
	min  slog.Level
	next slog.Handler
}

func (h levelHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.min && h.next.Enabled(ctx, level)
}

func (h levelHandler) Handle(ctx context.Context, r slog.Record) error {
	return h.next.Handle(ctx, r)
}

func (h levelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return levelHandler{min: h.min, next: h.next.WithAttrs(attrs)}
}

func (h levelHandler) WithGroup(name string) slog.Handler {
	return levelHandler{min: h.min, next: h.next.WithGroup(name)}
}

func parseLogLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
