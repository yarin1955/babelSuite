package queue

import (
	"context"
	"errors"
	"sync"
	"time"
)

var ErrDuplicateTask = errors.New("duplicate task")

type Task struct {
	ID           string
	Group        string
	Name         string
	Dependencies []string
	LeaseTTL     time.Duration
	Run          func(context.Context) error
}

type State string

const (
	StatePending   State = "pending"
	StateRunning   State = "running"
	StateSucceeded State = "succeeded"
	StateFailed    State = "failed"
	StateCanceled  State = "canceled"
)

type Entry struct {
	Task          Task
	State         State
	LeaseDeadline time.Time
	Err           error
	StartedAt     time.Time
	CompletedAt   time.Time
}

type Memory struct {
	ctx    context.Context
	cancel context.CancelFunc
	wakeCh chan struct{}

	mu      sync.Mutex
	order   []string
	entries map[string]*entry
}

type entry struct {
	Entry
	cancel context.CancelFunc
}

func NewMemory(parent context.Context, workers int) *Memory {
	if workers <= 0 {
		workers = 1
	}

	ctx, cancel := context.WithCancel(parent)
	m := &Memory{
		ctx:     ctx,
		cancel:  cancel,
		wakeCh:  make(chan struct{}, 1),
		entries: make(map[string]*entry),
	}

	for range workers {
		go m.worker()
	}
	go m.expiryLoop()
	registerQueueGauge(m)
	return m
}

func (m *Memory) Enqueue(tasks []Task) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, task := range tasks {
		if task.ID == "" {
			return errors.New("task id is required")
		}
		if task.Run == nil {
			return errors.New("task run callback is required")
		}
		if _, exists := m.entries[task.ID]; exists {
			return ErrDuplicateTask
		}
	}

	for _, task := range tasks {
		leaseTTL := task.LeaseTTL
		if leaseTTL <= 0 {
			leaseTTL = 10 * time.Second
		}

		m.entries[task.ID] = &entry{
			Entry: Entry{
				Task: Task{
					ID:           task.ID,
					Group:        task.Group,
					Name:         task.Name,
					Dependencies: append([]string{}, task.Dependencies...),
					LeaseTTL:     leaseTTL,
					Run:          task.Run,
				},
				State: StatePending,
			},
		}
		m.order = append(m.order, task.ID)
	}

	m.signal()
	return nil
}

func (m *Memory) CancelGroup(group string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, id := range m.order {
		item := m.entries[id]
		if item == nil || item.Task.Group != group {
			continue
		}

		switch item.State {
		case StatePending:
			item.State = StateCanceled
			item.CompletedAt = time.Now().UTC()
			item.Err = context.Canceled
		case StateRunning:
			if item.cancel != nil {
				item.cancel()
			}
		}
	}

	m.signal()
}

func (m *Memory) CancelRunningGroup(group string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, id := range m.order {
		item := m.entries[id]
		if item == nil || item.Task.Group != group {
			continue
		}
		if item.State == StateRunning && item.cancel != nil {
			item.cancel()
		}
	}
}

func (m *Memory) Snapshot() []Entry {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]Entry, 0, len(m.order))
	for _, id := range m.order {
		item := m.entries[id]
		if item == nil {
			continue
		}
		result = append(result, item.Entry)
	}
	return result
}

func (m *Memory) Close() {
	m.cancel()
}

func (m *Memory) stateTallies() map[State]int64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	counts := make(map[State]int64)
	for _, id := range m.order {
		item := m.entries[id]
		if item != nil {
			counts[item.State]++
		}
	}
	return counts
}

func (m *Memory) worker() {
	for {
		task, ok := m.claim()
		if !ok {
			select {
			case <-m.ctx.Done():
				return
			case <-m.wakeCh:
				continue
			}
		}

		spanCtx, span := startTaskSpan(m.ctx, task)
		runCtx, cancel := context.WithCancel(spanCtx)
		m.attachCancel(task.ID, cancel)

		doneCh := make(chan struct{})
		go m.renewLease(task.ID, task.LeaseTTL, doneCh)

		err := task.Run(runCtx)

		close(doneCh)
		cancel()
		finishTaskSpan(spanCtx, span, task, err)
		m.finish(task.ID, err)
	}
}

func (m *Memory) expiryLoop() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			now := time.Now().UTC()
			expired := make([]string, 0)

			m.mu.Lock()
			for id, item := range m.entries {
				if item.State == StateRunning && !item.LeaseDeadline.IsZero() && now.After(item.LeaseDeadline) {
					expired = append(expired, id)
					if item.cancel != nil {
						item.cancel()
					}
				}
			}
			m.mu.Unlock()

			for _, id := range expired {
				queueMetrics.leaseExpiries.Add(context.Background(), 1)
				m.finish(id, context.DeadlineExceeded)
			}
		}
	}
}

func (m *Memory) claim() (Task, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	for _, id := range m.order {
		item := m.entries[id]
		if item == nil || item.State != StatePending {
			continue
		}

		dependenciesReady := true
		for _, dependency := range item.Task.Dependencies {
			dep := m.entries[dependency]
			if dep == nil {
				item.State = StateCanceled
				item.CompletedAt = now
				item.Err = errors.New("dependency missing")
				dependenciesReady = false
				break
			}

			switch dep.State {
			case StatePending, StateRunning:
				dependenciesReady = false
			}
		}
		if !dependenciesReady {
			continue
		}

		item.State = StateRunning
		item.StartedAt = now
		item.LeaseDeadline = now.Add(item.Task.LeaseTTL)
		return item.Task, true
	}

	return Task{}, false
}

func (m *Memory) attachCancel(id string, cancel context.CancelFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()

	item := m.entries[id]
	if item == nil || item.State != StateRunning {
		cancel()
		return
	}

	item.cancel = cancel
}

func (m *Memory) renewLease(id string, ttl time.Duration, doneCh <-chan struct{}) {
	renewEvery := ttl / 2
	if renewEvery <= 0 {
		renewEvery = time.Second
	}

	ticker := time.NewTicker(renewEvery)
	defer ticker.Stop()

	for {
		select {
		case <-doneCh:
			return
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.mu.Lock()
			item := m.entries[id]
			if item != nil && item.State == StateRunning {
				item.LeaseDeadline = time.Now().UTC().Add(ttl)
			}
			m.mu.Unlock()
		}
	}
}

func (m *Memory) finish(id string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	item := m.entries[id]
	if item == nil {
		return
	}
	if item.State == StateSucceeded || item.State == StateFailed || item.State == StateCanceled {
		return
	}

	item.cancel = nil
	item.CompletedAt = time.Now().UTC()
	item.Err = err
	item.LeaseDeadline = time.Time{}

	switch {
	case errors.Is(err, context.Canceled):
		item.State = StateCanceled
	case err != nil:
		item.State = StateFailed
	default:
		item.State = StateSucceeded
	}

	m.signal()
}

func (m *Memory) signal() {
	select {
	case m.wakeCh <- struct{}{}:
	default:
	}
}
