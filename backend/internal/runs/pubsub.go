package runs

import (
	"sync"

	"github.com/babelsuite/babelsuite/internal/domain"
)

// LogPubSub fans log entries out to any number of SSE subscribers
// keyed by step ID.
type LogPubSub struct {
	mu   sync.RWMutex
	subs map[string][]chan []*domain.LogEntry
}

func newLogPubSub() *LogPubSub {
	return &LogPubSub{subs: make(map[string][]chan []*domain.LogEntry)}
}

// Subscribe returns a channel that receives batches of log entries for stepID.
// Call the returned cancel function to unsubscribe.
func (p *LogPubSub) Subscribe(stepID string) (<-chan []*domain.LogEntry, func()) {
	ch := make(chan []*domain.LogEntry, 64)
	p.mu.Lock()
	p.subs[stepID] = append(p.subs[stepID], ch)
	p.mu.Unlock()

	cancel := func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		list := p.subs[stepID]
		for i, c := range list {
			if c == ch {
				p.subs[stepID] = append(list[:i], list[i+1:]...)
				break
			}
		}
		if len(p.subs[stepID]) == 0 {
			delete(p.subs, stepID)
		}
		close(ch)
	}
	return ch, cancel
}

// Publish sends entries to all subscribers of stepID (non-blocking).
func (p *LogPubSub) Publish(stepID string, entries []*domain.LogEntry) {
	p.mu.RLock()
	list := make([]chan []*domain.LogEntry, len(p.subs[stepID]))
	copy(list, p.subs[stepID])
	p.mu.RUnlock()

	for _, ch := range list {
		select {
		case ch <- entries:
		default:
		}
	}
}

// RunPubSub fans run-level status events to subscribers keyed by run ID.
type RunPubSub struct {
	mu   sync.RWMutex
	subs map[string][]chan *domain.Run
}

func newRunPubSub() *RunPubSub {
	return &RunPubSub{subs: make(map[string][]chan *domain.Run)}
}

func (p *RunPubSub) Subscribe(runID string) (<-chan *domain.Run, func()) {
	ch := make(chan *domain.Run, 8)
	p.mu.Lock()
	p.subs[runID] = append(p.subs[runID], ch)
	p.mu.Unlock()

	cancel := func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		list := p.subs[runID]
		for i, c := range list {
			if c == ch {
				p.subs[runID] = append(list[:i], list[i+1:]...)
				break
			}
		}
		if len(p.subs[runID]) == 0 {
			delete(p.subs, runID)
		}
		close(ch)
	}
	return ch, cancel
}

func (p *RunPubSub) Publish(run *domain.Run) {
	p.mu.RLock()
	list := make([]chan *domain.Run, len(p.subs[run.RunID]))
	copy(list, p.subs[run.RunID])
	p.mu.RUnlock()

	for _, ch := range list {
		select {
		case ch <- run:
		default:
		}
	}
}
