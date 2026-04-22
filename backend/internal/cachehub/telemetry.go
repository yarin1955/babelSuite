package cachehub

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const cacheScope = "github.com/babelsuite/babelsuite/internal/cachehub"

type cacheSignals struct {
	tracer metric.Int64Counter
	gets   metric.Int64Counter
	sets   metric.Int64Counter
	dels   metric.Int64Counter
}

var cacheTracer = otel.Tracer(cacheScope)

func init() {
	meter := otel.Meter(cacheScope)
	cacheGets, _ = meter.Int64Counter("babelsuite.cache.gets",
		metric.WithDescription("Redis GET operations"))
	cacheSets, _ = meter.Int64Counter("babelsuite.cache.sets",
		metric.WithDescription("Redis SET operations"))
	cacheDels, _ = meter.Int64Counter("babelsuite.cache.deletes",
		metric.WithDescription("Redis DEL operations"))
}

var (
	cacheGets metric.Int64Counter
	cacheSets metric.Int64Counter
	cacheDels metric.Int64Counter
)

func startCacheSpan(ctx context.Context, op string) (context.Context, trace.Span) {
	return cacheTracer.Start(ctx, "cache."+op,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("db.system", "redis"),
			attribute.String("db.operation.name", op),
		),
	)
}

func endCacheSpan(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}

func recordCacheGet(ctx context.Context, hit bool) {
	cacheGets.Add(ctx, 1, metric.WithAttributes(attribute.Bool("cache.hit", hit)))
}

func recordCacheSet(ctx context.Context) {
	cacheSets.Add(ctx, 1)
}

func recordCacheDel(ctx context.Context) {
	cacheDels.Add(ctx, 1)
}
