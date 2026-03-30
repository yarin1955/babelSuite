package platform

import (
	"context"
	"time"

	"github.com/babelsuite/babelsuite/internal/cachehub"
)

type cacheStore struct {
	base Store
	hub  *cachehub.Hub
	ttl  time.Duration
}

func WithRedis(base Store, hub *cachehub.Hub, ttl time.Duration) Store {
	if base == nil || hub == nil || !hub.Enabled() {
		return base
	}
	return &cacheStore{
		base: base,
		hub:  hub,
		ttl:  ttl,
	}
}

func (s *cacheStore) Load() (*PlatformSettings, error) {
	var cached PlatformSettings
	if ok, err := s.hub.ReadJSON(context.Background(), s.cacheKey(), &cached); err == nil && ok {
		return &cached, nil
	}

	settings, err := s.base.Load()
	if err != nil {
		return nil, err
	}
	_ = s.hub.WriteJSON(context.Background(), s.cacheKey(), settings, s.ttl)
	return settings, nil
}

func (s *cacheStore) Save(settings *PlatformSettings) error {
	if err := s.base.Save(settings); err != nil {
		return err
	}

	fresh, err := s.base.Load()
	if err != nil {
		return err
	}
	_ = s.hub.WriteJSON(context.Background(), s.cacheKey(), fresh, s.ttl)
	_ = s.hub.BumpScope(context.Background(), "catalog")
	return nil
}

func (s *cacheStore) SyncRegistry(registryID string) (*PlatformSettings, error) {
	settings, err := s.base.SyncRegistry(registryID)
	if err != nil {
		return nil, err
	}

	_ = s.hub.WriteJSON(context.Background(), s.cacheKey(), settings, s.ttl)
	_ = s.hub.BumpScope(context.Background(), "catalog")
	return settings, nil
}

func (s *cacheStore) cacheKey() string {
	return s.hub.Key("platform", "settings")
}
