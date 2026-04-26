package catalog

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const catalogScope = "github.com/babelsuite/babelsuite/internal/catalog"

type catalogSignals struct {
	cacheHits   metric.Int64Counter
	cacheMisses metric.Int64Counter
	fetchCount  metric.Int64Counter
}

func newCatalogSignals() *catalogSignals {
	meter := otel.Meter(catalogScope)
	hits, _ := meter.Int64Counter("babelsuite.catalog.cache.hits",
		metric.WithDescription("Cache hits for catalog reads"))
	misses, _ := meter.Int64Counter("babelsuite.catalog.cache.misses",
		metric.WithDescription("Cache misses for catalog reads"))
	fetches, _ := meter.Int64Counter("babelsuite.catalog.registry.fetches",
		metric.WithDescription("OCI registry HTTP fetch count"))
	return &catalogSignals{
		cacheHits:   hits,
		cacheMisses: misses,
		fetchCount:  fetches,
	}
}

var catalogMetrics = newCatalogSignals()

func recordCacheHit(ctx context.Context, operation string) {
	catalogMetrics.cacheHits.Add(ctx, 1, metric.WithAttributes(
		attribute.String("operation", operation),
	))
}

func recordCacheMiss(ctx context.Context, operation string) {
	catalogMetrics.cacheMisses.Add(ctx, 1, metric.WithAttributes(
		attribute.String("operation", operation),
	))
}

func recordRegistryFetch(ctx context.Context, registry string) {
	catalogMetrics.fetchCount.Add(ctx, 1, metric.WithAttributes(
		attribute.String("registry", registry),
	))
}
