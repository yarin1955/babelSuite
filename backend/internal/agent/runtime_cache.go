package agent

import (
	"context"
	"time"

	"github.com/babelsuite/babelsuite/internal/cachehub"
)

const assignmentsCacheKey = "agent-runtime:assignments"

func (c *Coordinator) ConfigureStore(store AssignmentStore) {
	c.mu.Lock()
	c.store = store
	c.mu.Unlock()
	c.restoreAssignments()
}

func (c *Coordinator) ConfigureRuntimeCache(hub *cachehub.Hub, ttl time.Duration) {
	c.mu.Lock()
	c.hub = hub
	if ttl > 0 {
		c.ttl = ttl
	}
	c.mu.Unlock()
	c.restoreAssignments()
}

func (c *Coordinator) restoreAssignments() {
	var snapshots []AssignmentSnapshot
	if c != nil && c.store != nil {
		loaded, err := c.store.LoadAssignmentRuntime(context.Background())
		if err == nil && len(loaded) > 0 {
			snapshots = loaded
		}
	}

	if len(snapshots) == 0 && c != nil && c.hub != nil && c.hub.Enabled() {
		ok, err := c.hub.ReadJSON(context.Background(), assignmentsCacheKey, &snapshots)
		if err != nil || !ok {
			snapshots = nil
		}
	}
	if len(snapshots) == 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	for _, snapshot := range snapshots {
		request := snapshot.Request
		if request.JobID == "" {
			continue
		}
		c.assignments[request.JobID] = &assignmentEntry{
			request: request,
			status:  AssignmentPending,
			done:    make(chan struct{}),
		}
	}
}

func (c *Coordinator) persistAssignments() {
	c.mu.Lock()
	snapshots := make([]AssignmentSnapshot, 0, len(c.assignments))
	for _, entry := range c.assignments {
		if entry.completed {
			continue
		}
		snapshots = append(snapshots, AssignmentSnapshot{
			Request:         entry.request,
			Status:          entry.status,
			ClaimedBy:       entry.claimedBy,
			CancelRequested: entry.cancelRequested,
			LeaseDeadline:   entry.leaseDeadline,
			Completed:       entry.completed,
		})
	}
	store := c.store
	hub := c.hub
	ttl := c.ttl
	c.mu.Unlock()

	if store != nil {
		saveCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = store.SaveAssignmentRuntime(saveCtx, snapshots)
		cancel()
	}
	if hub == nil || !hub.Enabled() {
		return
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	_ = hub.WriteJSON(context.Background(), assignmentsCacheKey, snapshots, ttl)
}
