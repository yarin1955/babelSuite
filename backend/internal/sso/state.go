package sso

import (
	"sync"
	"time"
)

type stateEntry struct {
	providerID string
	returnURL  string
	expiry     time.Time
}

type stateStore struct {
	mu      sync.Mutex
	entries map[string]stateEntry
}

func newStateStore() *stateStore {
	s := &stateStore{entries: make(map[string]stateEntry)}
	go s.cleanup()
	return s
}

func (s *stateStore) put(nonce, providerID, returnURL string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[nonce] = stateEntry{
		providerID: providerID,
		returnURL:  returnURL,
		expiry:     time.Now().Add(10 * time.Minute),
	}
}

func (s *stateStore) pop(nonce string) (stateEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[nonce]
	if !ok || time.Now().After(e.expiry) {
		delete(s.entries, nonce)
		return stateEntry{}, false
	}
	delete(s.entries, nonce)
	return e, true
}

func (s *stateStore) cleanup() {
	for range time.Tick(5 * time.Minute) {
		s.mu.Lock()
		now := time.Now()
		for k, e := range s.entries {
			if now.After(e.expiry) {
				delete(s.entries, k)
			}
		}
		s.mu.Unlock()
	}
}
