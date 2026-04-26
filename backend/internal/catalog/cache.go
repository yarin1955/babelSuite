package catalog

import (
	"context"
	"time"

	"github.com/babelsuite/babelsuite/internal/cachehub"
)

type cachedReader struct {
	base Reader
	hub  *cachehub.Hub
	ttl  time.Duration
}

func WithRedis(base Reader, hub *cachehub.Hub, ttl time.Duration) Reader {
	if base == nil || hub == nil || !hub.Enabled() {
		return base
	}
	return &cachedReader{
		base: base,
		hub:  hub,
		ttl:  ttl,
	}
}

func (r *cachedReader) ListPackages() ([]Package, error) {
	ctx := context.Background()
	key := r.hub.Key("catalog", r.hub.ScopeStamp(ctx, "catalog"), "packages")

	var cached []Package
	if ok, err := r.hub.ReadJSON(ctx, key, &cached); err == nil && ok {
		recordCacheHit(ctx, "list_packages")
		return clonePackages(cached), nil
	}
	recordCacheMiss(ctx, "list_packages")

	packages, err := r.base.ListPackages()
	if err != nil {
		return nil, err
	}
	_ = r.hub.WriteJSON(ctx, key, packages, r.ttl)
	return packages, nil
}

func (r *cachedReader) GetPackage(id string) (*Package, error) {
	ctx := context.Background()
	key := r.hub.Key("catalog", r.hub.ScopeStamp(ctx, "catalog"), "package", id)

	var cached Package
	if ok, err := r.hub.ReadJSON(ctx, key, &cached); err == nil && ok {
		recordCacheHit(ctx, "get_package")
		cached.Tags = append([]string{}, cached.Tags...)
		cached.Modules = append([]string{}, cached.Modules...)
		return &cached, nil
	}
	recordCacheMiss(ctx, "get_package")

	item, err := r.base.GetPackage(id)
	if err != nil {
		return nil, err
	}
	_ = r.hub.WriteJSON(ctx, key, item, r.ttl)
	return item, nil
}
