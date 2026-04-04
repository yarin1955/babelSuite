package agent

import (
	"context"
	"errors"

	"github.com/babelsuite/babelsuite/internal/logstream"
)

type LocalDispatcher struct {
	service *Service
}

func NewLocalDispatcher(service *Service) *LocalDispatcher {
	return &LocalDispatcher{service: service}
}

func (d *LocalDispatcher) IsAvailable(context.Context) bool {
	return d != nil && d.service != nil
}

func (d *LocalDispatcher) Dispatch(ctx context.Context, request StepRequest, emit func(logstream.Line)) error {
	if d == nil || d.service == nil {
		return errors.New("local dispatcher is not configured")
	}

	var dispatchErr error
	d.service.Stream(ctx, request, func(message StreamMessage) {
		if message.Type == "log" && message.Line != nil && emit != nil {
			emit(*message.Line)
		}
		if message.Type == "done" && message.Error != "" {
			dispatchErr = errors.New(message.Error)
		}
	})
	return dispatchErr
}
