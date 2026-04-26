package telemetry

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc/credentials"
)

type Pipeline struct {
	traceProvider *sdktrace.TracerProvider
	meterProvider *sdkmetric.MeterProvider
	logProvider   *sdklog.LoggerProvider
}

func Start(ctx context.Context) (*Pipeline, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	endpoint, schemeHint := normalizeCollectorEndpoint(strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")))
	if endpoint == "" {
		return &Pipeline{}, nil
	}

	serviceName := strings.TrimSpace(os.Getenv("OTEL_SERVICE_NAME"))
	if serviceName == "" {
		serviceName = "babelsuite-backend"
	}

	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
			semconv.ServiceNamespaceKey.String("babelsuite"),
		),
	)
	if err != nil {
		return nil, err
	}

	sharedHeaders := readCollectorHeaders()
	insecure := shouldSkipTLS(endpoint, schemeHint)
	protocol := strings.ToLower(strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL")))

	var traceExporter sdktrace.SpanExporter
	var metricExporter sdkmetric.Exporter
	var logExporter sdklog.Exporter

	switch protocol {
	case "http/protobuf", "http":
		traceExporter, err = buildHTTPTraceExporter(ctx, endpoint, insecure, sharedHeaders)
		if err != nil {
			return nil, err
		}
		metricExporter, err = buildHTTPMetricExporter(ctx, endpoint, insecure, sharedHeaders)
		if err != nil {
			_ = traceExporter.Shutdown(ctx)
			return nil, err
		}
		logExporter, err = buildHTTPLogExporter(ctx, endpoint, insecure, sharedHeaders)
		if err != nil {
			_ = traceExporter.Shutdown(ctx)
			_ = metricExporter.Shutdown(ctx)
			return nil, err
		}
	default:
		traceExporter, err = otlptracegrpc.New(ctx, traceGRPCOptions(endpoint, insecure, sharedHeaders)...)
		if err != nil {
			return nil, err
		}
		metricExporter, err = otlpmetricgrpc.New(ctx, metricGRPCOptions(endpoint, insecure, sharedHeaders)...)
		if err != nil {
			_ = traceExporter.Shutdown(ctx)
			return nil, err
		}
		logExporter, err = buildGRPCLogExporter(ctx, endpoint, insecure, sharedHeaders)
		if err != nil {
			_ = traceExporter.Shutdown(ctx)
			_ = metricExporter.Shutdown(ctx)
			return nil, err
		}
	}

	traceProvider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSampler(buildSampler()),
		sdktrace.WithBatcher(traceExporter),
	)
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter, sdkmetric.WithInterval(15*time.Second))),
	)
	logProvider := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)),
	)

	otel.SetTracerProvider(traceProvider)
	otel.SetMeterProvider(meterProvider)
	global.SetLoggerProvider(logProvider)

	return &Pipeline{
		traceProvider: traceProvider,
		meterProvider: meterProvider,
		logProvider:   logProvider,
	}, nil
}

func (p *Pipeline) Enabled() bool {
	return p != nil && p.traceProvider != nil
}

func (p *Pipeline) Shutdown(ctx context.Context) error {
	if p == nil {
		return nil
	}
	var combined error
	if p.logProvider != nil {
		combined = errors.Join(combined, p.logProvider.Shutdown(ctx))
	}
	if p.meterProvider != nil {
		combined = errors.Join(combined, p.meterProvider.Shutdown(ctx))
	}
	if p.traceProvider != nil {
		combined = errors.Join(combined, p.traceProvider.Shutdown(ctx))
	}
	return combined
}

func (p *Pipeline) LogProvider() *sdklog.LoggerProvider {
	if p == nil {
		return nil
	}
	return p.logProvider
}

func WrapHTTP(next http.Handler) http.Handler {
	if next == nil {
		return http.NotFoundHandler()
	}
	return otelhttp.NewHandler(next, "babelsuite.http",
		otelhttp.WithFilter(func(r *http.Request) bool {
			return r.Method != http.MethodOptions
		}),
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			path := strings.TrimSpace(r.URL.Path)
			if path == "" {
				path = "/"
			}
			return r.Method + " " + path
		}),
	)
}

func buildSampler() sdktrace.Sampler {
	name := strings.ToLower(strings.TrimSpace(os.Getenv("OTEL_TRACES_SAMPLER")))
	arg := strings.TrimSpace(os.Getenv("OTEL_TRACES_SAMPLER_ARG"))
	switch name {
	case "always_off":
		return sdktrace.NeverSample()
	case "traceidratio":
		return sdktrace.TraceIDRatioBased(parseSamplerRatio(arg))
	case "parentbased_always_off":
		return sdktrace.ParentBased(sdktrace.NeverSample())
	case "parentbased_traceidratio":
		return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(parseSamplerRatio(arg)))
	default:
		return sdktrace.ParentBased(sdktrace.AlwaysSample())
	}
}

func parseSamplerRatio(s string) float64 {
	if v, err := strconv.ParseFloat(s, 64); err == nil && v >= 0 && v <= 1 {
		return v
	}
	return 1.0
}

func buildHTTPTraceExporter(ctx context.Context, endpoint string, insecure bool, headers map[string]string) (sdktrace.SpanExporter, error) {
	opts := []otlptracehttp.Option{otlptracehttp.WithEndpoint(endpoint)}
	if len(headers) > 0 {
		opts = append(opts, otlptracehttp.WithHeaders(headers))
	}
	if insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}
	return otlptracehttp.New(ctx, opts...)
}

func buildHTTPMetricExporter(ctx context.Context, endpoint string, insecure bool, headers map[string]string) (sdkmetric.Exporter, error) {
	opts := []otlpmetrichttp.Option{otlpmetrichttp.WithEndpoint(endpoint)}
	if len(headers) > 0 {
		opts = append(opts, otlpmetrichttp.WithHeaders(headers))
	}
	if insecure {
		opts = append(opts, otlpmetrichttp.WithInsecure())
	}
	return otlpmetrichttp.New(ctx, opts...)
}

func buildHTTPLogExporter(ctx context.Context, endpoint string, insecure bool, headers map[string]string) (sdklog.Exporter, error) {
	opts := []otlploghttp.Option{otlploghttp.WithEndpoint(endpoint)}
	if len(headers) > 0 {
		opts = append(opts, otlploghttp.WithHeaders(headers))
	}
	if insecure {
		opts = append(opts, otlploghttp.WithInsecure())
	}
	return otlploghttp.New(ctx, opts...)
}

func buildGRPCLogExporter(ctx context.Context, endpoint string, insecure bool, headers map[string]string) (sdklog.Exporter, error) {
	opts := []otlploggrpc.Option{otlploggrpc.WithEndpoint(endpoint)}
	if len(headers) > 0 {
		opts = append(opts, otlploggrpc.WithHeaders(headers))
	}
	if insecure {
		return otlploggrpc.New(ctx, append(opts, otlploggrpc.WithInsecure())...)
	}
	return otlploggrpc.New(ctx, append(opts, otlploggrpc.WithTLSCredentials(credentials.NewClientTLSFromCert(nil, "")))...)
}

func traceGRPCOptions(endpoint string, insecure bool, headers map[string]string) []otlptracegrpc.Option {
	opts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(endpoint)}
	if len(headers) > 0 {
		opts = append(opts, otlptracegrpc.WithHeaders(headers))
	}
	if insecure {
		return append(opts, otlptracegrpc.WithInsecure())
	}
	return append(opts, otlptracegrpc.WithTLSCredentials(credentials.NewClientTLSFromCert(nil, "")))
}

func metricGRPCOptions(endpoint string, insecure bool, headers map[string]string) []otlpmetricgrpc.Option {
	opts := []otlpmetricgrpc.Option{otlpmetricgrpc.WithEndpoint(endpoint)}
	if len(headers) > 0 {
		opts = append(opts, otlpmetricgrpc.WithHeaders(headers))
	}
	if insecure {
		return append(opts, otlpmetricgrpc.WithInsecure())
	}
	return append(opts, otlpmetricgrpc.WithTLSCredentials(credentials.NewClientTLSFromCert(nil, "")))
}

func normalizeCollectorEndpoint(raw string) (string, bool) {
	switch {
	case strings.HasPrefix(raw, "http://"):
		return strings.TrimPrefix(raw, "http://"), true
	case strings.HasPrefix(raw, "https://"):
		return strings.TrimPrefix(raw, "https://"), false
	default:
		return raw, false
	}
}

func shouldSkipTLS(endpoint string, schemeHint bool) bool {
	raw := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_INSECURE"))
	if raw != "" {
		if enabled, err := strconv.ParseBool(raw); err == nil {
			return enabled
		}
	}
	if schemeHint {
		return true
	}
	host := strings.ToLower(endpoint)
	return strings.HasPrefix(host, "localhost:") ||
		strings.HasPrefix(host, "127.0.0.1:") ||
		strings.HasPrefix(host, "[::1]:")
}

func readCollectorHeaders() map[string]string {
	raw := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_HEADERS"))
	if raw == "" {
		return nil
	}
	headers := make(map[string]string)
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		headers[key] = value
	}
	if len(headers) == 0 {
		return nil
	}
	return headers
}
