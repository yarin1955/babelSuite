package agent

import (
	"context"
	"errors"

	"github.com/babelsuite/babelsuite/internal/logstream"
)

type BrokerDispatcher struct {
	backendID   string
	registry    RegistryReader
	coordinator *Coordinator
}

func NewBrokerDispatcher(backendID string, registry RegistryReader, coordinator *Coordinator) *BrokerDispatcher {
	return &BrokerDispatcher{
		backendID:   backendID,
		registry:    registry,
		coordinator: coordinator,
	}
}

func (d *BrokerDispatcher) IsAvailable(context.Context) bool {
	if d == nil || d.registry == nil {
		return false
	}
	if registry, ok := d.registry.(*Registry); ok && registry == nil {
		return false
	}
	return d.registry.IsAvailable(d.backendID)
}

func (d *BrokerDispatcher) Dispatch(ctx context.Context, request StepRequest, _ func(logstream.Line)) error {
	if d == nil || d.coordinator == nil {
		return errors.New("remote broker is not configured")
	}

	request.BackendID = firstNonEmpty(request.BackendID, d.backendID)
	assignment, err := d.coordinator.Submit(request)
	if err != nil {
		return err
	}
	defer d.coordinator.Cleanup(assignment.JobID)

	return d.coordinator.Wait(ctx, assignment.JobID)
}
