package profiles

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

func (s *cacheStore) Load() (*Document, error) {
	var cached Document
	if ok, err := s.hub.ReadJSON(context.Background(), s.cacheKey(), &cached); err == nil && ok {
		normalizeDocument(&cached)
		return &cached, nil
	}

	document, err := s.base.Load()
	if err != nil {
		return nil, err
	}
	_ = s.hub.WriteJSON(context.Background(), s.cacheKey(), document, s.ttl)
	return document, nil
}

func (s *cacheStore) Save(document *Document) error {
	if err := s.base.Save(document); err != nil {
		return err
	}

	fresh, err := s.base.Load()
	if err != nil {
		return err
	}
	_ = s.hub.WriteJSON(context.Background(), s.cacheKey(), fresh, s.ttl)
	return nil
}

func (s *cacheStore) cacheKey() string {
	return s.hub.Key("profiles", "document")
}
