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
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc/credentials"
)

type Pipeline struct {
	traceProvider *sdktrace.TracerProvider
	meterProvider *sdkmetric.MeterProvider
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
			attribute.String("service.name", serviceName),
			attribute.String("service.namespace", "babelsuite"),
		),
	)
	if err != nil {
		return nil, err
	}

	sharedHeaders := readCollectorHeaders()
	insecureTransport := shouldSkipTLS(endpoint, schemeHint)

	traceExporter, err := otlptracegrpc.New(ctx, traceClientOptions(endpoint, insecureTransport, sharedHeaders)...)
	if err != nil {
		return nil, err
	}

	metricExporter, err := otlpmetricgrpc.New(ctx, metricClientOptions(endpoint, insecureTransport, sharedHeaders)...)
	if err != nil {
		_ = traceExporter.Shutdown(ctx)
		return nil, err
	}

	traceProvider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample())),
		sdktrace.WithBatcher(traceExporter),
	)
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter, sdkmetric.WithInterval(15*time.Second))),
	)

	otel.SetTracerProvider(traceProvider)
	otel.SetMeterProvider(meterProvider)

	return &Pipeline{
		traceProvider: traceProvider,
		meterProvider: meterProvider,
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
	if p.meterProvider != nil {
		combined = errors.Join(combined, p.meterProvider.Shutdown(ctx))
	}
	if p.traceProvider != nil {
		combined = errors.Join(combined, p.traceProvider.Shutdown(ctx))
	}
	return combined
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

func traceClientOptions(endpoint string, insecureTransport bool, headers map[string]string) []otlptracegrpc.Option {
	options := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(endpoint),
	}
	if len(headers) > 0 {
		options = append(options, otlptracegrpc.WithHeaders(headers))
	}
	if insecureTransport {
		return append(options, otlptracegrpc.WithInsecure())
	}
	return append(options, otlptracegrpc.WithTLSCredentials(credentials.NewClientTLSFromCert(nil, "")))
}

func metricClientOptions(endpoint string, insecureTransport bool, headers map[string]string) []otlpmetricgrpc.Option {
	options := []otlpmetricgrpc.Option{
		otlpmetricgrpc.WithEndpoint(endpoint),
	}
	if len(headers) > 0 {
		options = append(options, otlpmetricgrpc.WithHeaders(headers))
	}
	if insecureTransport {
		return append(options, otlpmetricgrpc.WithInsecure())
	}
	return append(options, otlpmetricgrpc.WithTLSCredentials(credentials.NewClientTLSFromCert(nil, "")))
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
		enabled, err := strconv.ParseBool(raw)
		if err == nil {
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
