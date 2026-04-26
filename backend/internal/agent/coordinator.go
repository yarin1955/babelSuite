package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/metric"

	"github.com/babelsuite/babelsuite/internal/logstream"
)

var (
	ErrAssignmentNotFound = errors.New("assignment not found")
	ErrAssignmentClaimed  = errors.New("assignment already claimed")
	ErrAssignmentClosed   = errors.New("assignment already completed")
)

type AssignmentObserver interface {
	AssignmentState(request StepRequest, state string, message string)
	AssignmentLog(request StepRequest, line logstream.Line)
}

type Coordinator struct {
	registry RegistryReader
	observer AssignmentObserver
	store    AssignmentStore
	hub      runtimeCache
	ttl      time.Duration

	mu          sync.Mutex
	assignments map[string]*assignmentEntry
}

type runtimeCache interface {
	Enabled() bool
	ReadJSON(ctx context.Context, key string, target any) (bool, error)
	WriteJSON(ctx context.Context, key string, value any, ttl time.Duration) error
}

type RegistryReader interface {
	IsAvailable(agentID string) bool
}

type assignmentEntry struct {
	request         StepRequest
	status          AssignmentStatus
	claimedBy       string
	cancelRequested bool
	leaseDeadline   time.Time
	err             error
	done            chan struct{}
	completed       bool
}

func NewCoordinator(registry RegistryReader, observer AssignmentObserver) *Coordinator {
	c := &Coordinator{
		registry:    registry,
		observer:    observer,
		assignments: make(map[string]*assignmentEntry),
	}
	go c.expiryLoop()
	registerCoordinatorGauge(c)
	return c
}

func (c *Coordinator) Submit(request StepRequest) (StepRequest, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	request.JobID = firstNonEmptyValue(strings.TrimSpace(request.JobID), request.ExecutionID+":"+request.Node.ID, uuid.NewString())
	if request.LeaseTTL <= 0 {
		request.LeaseTTL = 8 * time.Second
	}
	if _, exists := c.assignments[request.JobID]; exists {
		return StepRequest{}, fmt.Errorf("job %q already exists", request.JobID)
	}

	entry := &assignmentEntry{
		request: request,
		status:  AssignmentPending,
		done:    make(chan struct{}),
	}
	c.assignments[request.JobID] = entry
	go c.persistAssignments()
	return request, nil
}

func (c *Coordinator) Wait(ctx context.Context, jobID string) error {
	entry, ok := c.lookup(jobID)
	if !ok {
		return ErrAssignmentNotFound
	}

	select {
	case <-ctx.Done():
		c.Cancel(jobID)
		return ctx.Err()
	case <-entry.done:
		c.mu.Lock()
		defer c.mu.Unlock()
		current := c.assignments[jobID]
		if current == nil {
			return nil
		}
		return current.err
	}
}

func (c *Coordinator) Cleanup(jobID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.assignments, jobID)
	go c.persistAssignments()
}

func (c *Coordinator) Claim(agentID string) (*StepRequest, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now().UTC()
	for _, entry := range c.assignments {
		if entry.status != AssignmentPending {
			continue
		}
		if strings.TrimSpace(entry.request.BackendID) != strings.TrimSpace(agentID) {
			continue
		}
		entry.status = AssignmentRunning
		entry.claimedBy = agentID
		entry.leaseDeadline = now.Add(entry.request.LeaseTTL)
		requestCopy := entry.request
		go c.persistAssignments()
		return &requestCopy, true
	}
	return nil, false
}

func (c *Coordinator) Extend(jobID, agentID string) (LeaseResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.assignments[jobID]
	if !ok {
		return LeaseResponse{}, ErrAssignmentNotFound
	}
	if entry.status != AssignmentRunning {
		return LeaseResponse{Status: entry.status, CancelRequested: entry.cancelRequested}, nil
	}
	if entry.claimedBy != agentID {
		return LeaseResponse{}, ErrAssignmentClaimed
	}

	entry.leaseDeadline = time.Now().UTC().Add(entry.request.LeaseTTL)
	go c.persistAssignments()
	return LeaseResponse{
		Status:          entry.status,
		CancelRequested: entry.cancelRequested,
	}, nil
}

func (c *Coordinator) ReportState(jobID string, report StateReport) error {
	request, err := c.assignmentRequest(jobID, report.AgentID)
	if err != nil {
		return err
	}
	if c.observer != nil {
		c.observer.AssignmentState(request, report.State, report.Message)
	}
	return nil
}

func (c *Coordinator) ReportLog(jobID string, report LogReport) error {
	request, err := c.assignmentRequest(jobID, report.AgentID)
	if err != nil {
		return err
	}
	if c.observer != nil {
		c.observer.AssignmentLog(request, report.Line)
	}
	return nil
}

func (c *Coordinator) Complete(jobID string, request CompleteRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.assignments[jobID]
	if !ok {
		return ErrAssignmentNotFound
	}
	if entry.claimedBy != request.AgentID {
		return ErrAssignmentClaimed
	}
	if entry.completed {
		return nil
	}

	entry.completed = true
	entry.leaseDeadline = time.Time{}
	switch {
	case entry.cancelRequested:
		entry.status = AssignmentCanceled
		entry.err = context.Canceled
	case strings.TrimSpace(request.Error) != "":
		entry.status = AssignmentFailed
		entry.err = errors.New(request.Error)
	default:
		entry.status = AssignmentSucceeded
		entry.err = nil
	}
	close(entry.done)
	go c.persistAssignments()
	return nil
}

func (c *Coordinator) Cancel(jobID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.assignments[jobID]
	if !ok || entry.completed {
		return false
	}

	if entry.status == AssignmentPending {
		entry.completed = true
		entry.status = AssignmentCanceled
		entry.err = context.Canceled
		close(entry.done)
		go c.persistAssignments()
		return true
	}

	entry.cancelRequested = true
	go c.persistAssignments()
	return true
}

func (c *Coordinator) lookup(jobID string) (*assignmentEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.assignments[jobID]
	return entry, ok
}

func (c *Coordinator) assignmentRequest(jobID, agentID string) (StepRequest, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.assignments[jobID]
	if !ok {
		return StepRequest{}, ErrAssignmentNotFound
	}
	if entry.claimedBy != agentID {
		return StepRequest{}, ErrAssignmentClaimed
	}
	if entry.completed {
		return StepRequest{}, ErrAssignmentClosed
	}
	return entry.request, nil
}

func (c *Coordinator) expiryLoop() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		now := time.Now().UTC()
		for _, entry := range c.assignments {
			if entry.completed || entry.status != AssignmentRunning {
				continue
			}
			if entry.leaseDeadline.IsZero() || now.Before(entry.leaseDeadline) {
				continue
			}
			entry.completed = true
			entry.status = AssignmentFailed
			entry.err = context.DeadlineExceeded
			close(entry.done)
			agentMetrics.leaseExpiries.Add(context.Background(), 1, metric.WithAttributes(jobAttributes(entry.request)...))
		}
		c.mu.Unlock()
		c.persistAssignments()
	}
}

func (c *Coordinator) statusTallies() map[string]int64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	counts := make(map[string]int64)
	for _, entry := range c.assignments {
		counts[string(entry.status)]++
	}
	return counts
}

func firstNonEmptyValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
