package auth

import (
	"net"
	"net/http"
	"sync"
	"time"
)

const rateLimiterSweepInterval = 500

type ipRateLimiter struct {
	mu        sync.Mutex
	events    map[string][]time.Time
	limit     int
	window    time.Duration
	callCount int
}

func newIPRateLimiter(limit int, window time.Duration) *ipRateLimiter {
	return &ipRateLimiter{
		events: make(map[string][]time.Time),
		limit:  limit,
		window: window,
	}
}

func (l *ipRateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.callCount++
	if l.callCount >= rateLimiterSweepInterval {
		l.sweepLocked()
		l.callCount = 0
	}

	now := time.Now()
	cutoff := now.Add(-l.window)

	var fresh []time.Time
	for _, t := range l.events[key] {
		if t.After(cutoff) {
			fresh = append(fresh, t)
		}
	}

	if len(fresh) >= l.limit {
		l.events[key] = fresh
		return false
	}

	l.events[key] = append(fresh, now)
	return true
}

func (l *ipRateLimiter) sweepLocked() {
	cutoff := time.Now().Add(-l.window)
	for k, events := range l.events {
		var fresh []time.Time
		for _, t := range events {
			if t.After(cutoff) {
				fresh = append(fresh, t)
			}
		}
		if len(fresh) == 0 {
			delete(l.events, k)
		} else {
			l.events[k] = fresh
		}
	}
}

func (l *ipRateLimiter) middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !l.allow(remoteIP(r)) {
				writeSessionError(w, http.StatusTooManyRequests, "Too many requests. Please try again later.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
