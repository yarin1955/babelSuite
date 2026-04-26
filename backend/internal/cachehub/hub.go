package cachehub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type Options struct {
	Address  string
	Password string
	DB       int
	Prefix   string
}

type Hub struct {
	client *redis.Client
	prefix string
}

func New(options Options) (*Hub, error) {
	address := strings.TrimSpace(options.Address)
	if address == "" {
		return &Hub{}, nil
	}

	client := redis.NewClient(&redis.Options{
		Addr:     address,
		Password: options.Password,
		DB:       options.DB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}

	return &Hub{
		client: client,
		prefix: strings.TrimSpace(options.Prefix),
	}, nil
}

func (h *Hub) Enabled() bool {
	return h != nil && h.client != nil
}

func (h *Hub) Close() error {
	if !h.Enabled() {
		return nil
	}
	return h.client.Close()
}

func (h *Hub) Ping(ctx context.Context) error {
	if !h.Enabled() {
		return nil
	}
	return h.client.Ping(ctx).Err()
}

func (h *Hub) ReadJSON(ctx context.Context, key string, target any) (bool, error) {
	if !h.Enabled() {
		return false, nil
	}

	spanCtx, span := startCacheSpan(ctx, "get")
	defer span.End()

	payload, err := h.client.Get(spanCtx, h.compose(key)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			recordCacheGet(spanCtx, false)
			return false, nil
		}
		endCacheSpan(span, err)
		return false, err
	}

	if err := json.Unmarshal(payload, target); err != nil {
		_ = h.client.Del(spanCtx, h.compose(key)).Err()
		endCacheSpan(span, err)
		return false, err
	}
	recordCacheGet(spanCtx, true)
	return true, nil
}

func (h *Hub) WriteJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	if !h.Enabled() {
		return nil
	}

	spanCtx, span := startCacheSpan(ctx, "set")
	payload, err := json.Marshal(value)
	if err != nil {
		endCacheSpan(span, err)
		return err
	}
	err = h.client.Set(spanCtx, h.compose(key), payload, ttl).Err()
	endCacheSpan(span, err)
	recordCacheSet(spanCtx)
	return err
}

func (h *Hub) Remove(ctx context.Context, key string) error {
	if !h.Enabled() {
		return nil
	}
	spanCtx, span := startCacheSpan(ctx, "del")
	err := h.client.Del(spanCtx, h.compose(key)).Err()
	endCacheSpan(span, err)
	recordCacheDel(spanCtx)
	return err
}

func (h *Hub) ScopeStamp(ctx context.Context, scope string) string {
	if !h.Enabled() {
		return "0"
	}

	value, err := h.client.Get(ctx, h.compose(scopeKey(scope))).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "0"
		}
		return "0"
	}
	return value
}

func (h *Hub) BumpScope(ctx context.Context, scope string) error {
	if !h.Enabled() {
		return nil
	}
	return h.client.Incr(ctx, h.compose(scopeKey(scope))).Err()
}

func (h *Hub) Key(parts ...string) string {
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			clean = append(clean, part)
		}
	}
	return strings.Join(clean, ":")
}

func (h *Hub) compose(key string) string {
	if h.prefix == "" {
		return key
	}
	return fmt.Sprintf("%s:%s", h.prefix, key)
}

func scopeKey(scope string) string {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		scope = "default"
	}
	return "scope:" + scope
}
