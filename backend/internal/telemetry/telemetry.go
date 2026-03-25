package telemetry

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type Config struct {
	ServiceName    string
	ServiceVersion string
}

func Setup(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	if disabled() || !enabled() {
		return func(context.Context) error { return nil }, nil
	}

	exporter, err := newExporter(ctx)
	if err != nil {
		return nil, err
	}

	attrs := []attribute.KeyValue{
		attribute.String("service.name", strings.TrimSpace(cfg.ServiceName)),
	}
	if version := strings.TrimSpace(cfg.ServiceVersion); version != "" {
		attrs = append(attrs, attribute.String("service.version", version))
	}

	resource, err := sdkresource.New(ctx, sdkresource.WithAttributes(attrs...))
	if err != nil {
		return nil, err
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(sampleRatio()))),
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource),
	)
	otel.SetTracerProvider(provider)

	return provider.Shutdown, nil
}

func WrapHandler(next http.Handler, name string) http.Handler {
	return otelhttp.NewHandler(next, name)
}

func WrapClient(client *http.Client) *http.Client {
	if client == nil {
		client = &http.Client{}
	}

	clone := *client
	baseTransport := clone.Transport
	if baseTransport == nil {
		baseTransport = http.DefaultTransport
	}
	clone.Transport = otelhttp.NewTransport(baseTransport)
	return &clone
}

func Start(ctx context.Context, tracerName, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return otel.Tracer(tracerName).Start(ctx, spanName, opts...)
}

func SpanIDsFromContext(ctx context.Context) (string, string) {
	spanCtx := trace.SpanContextFromContext(ctx)
	if !spanCtx.IsValid() {
		return "", ""
	}
	return spanCtx.TraceID().String(), spanCtx.SpanID().String()
}

func disabled() bool {
	return strings.EqualFold(os.Getenv("OTEL_SDK_DISABLED"), "true")
}

func enabled() bool {
	if strings.EqualFold(os.Getenv("BABELSUITE_OTEL_STDOUT"), "true") {
		return true
	}
	return strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")) != "" ||
		strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")) != ""
}

func newExporter(ctx context.Context) (sdktrace.SpanExporter, error) {
	if strings.EqualFold(os.Getenv("BABELSUITE_OTEL_STDOUT"), "true") {
		return stdouttrace.New(stdouttrace.WithPrettyPrint())
	}
	return otlptracehttp.New(ctx)
}

func sampleRatio() float64 {
	raw := strings.TrimSpace(os.Getenv("BABELSUITE_OTEL_SAMPLE_RATIO"))
	if raw == "" {
		return 1
	}

	ratio, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 1
	}
	if ratio < 0 {
		return 0
	}
	if ratio > 1 {
		return 1
	}
	return ratio
}
