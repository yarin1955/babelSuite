package agent

import (
	"context"
	"errors"
	"time"

	"github.com/babelsuite/babelsuite/internal/logstream"
)

type Worker struct {
	agentID      string
	pollInterval time.Duration
	controlPlane *ControlPlaneClient
	service      *Service
}

func NewWorker(agentID string, pollInterval time.Duration, controlPlane *ControlPlaneClient, service *Service) *Worker {
	if pollInterval <= 0 {
		pollInterval = time.Second
	}
	return &Worker{
		agentID:      agentID,
		pollInterval: pollInterval,
		controlPlane: controlPlane,
		service:      service,
	}
}

func (w *Worker) Run(ctx context.Context) error {
	if w == nil || w.controlPlane == nil || w.service == nil {
		return errors.New("worker is not configured")
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		claimCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		assignment, err := w.controlPlane.ClaimNext(claimCtx, w.agentID)
		cancel()
		if err != nil {
			select {
			case <-time.After(w.pollInterval):
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if assignment == nil {
			select {
			case <-time.After(w.pollInterval):
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		w.runAssignment(ctx, *assignment)
	}
}

func (w *Worker) runAssignment(ctx context.Context, request StepRequest) {
	_ = w.controlPlane.ReportState(ctx, request.JobID, StateReport{
		AgentID: w.agentID,
		State:   "claimed",
	})
	_ = w.controlPlane.ReportState(ctx, request.JobID, StateReport{
		AgentID: w.agentID,
		State:   "running",
	})

	jobCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	leaseDone := make(chan struct{})
	go w.extendLease(jobCtx, request, cancel, leaseDone)

	var runErr error
	w.service.Stream(jobCtx, request, func(message StreamMessage) {
		switch message.Type {
		case "log":
			if message.Line != nil {
				_ = w.controlPlane.ReportLog(jobCtx, request.JobID, w.agentID, *message.Line)
			}
		case "done":
			if message.Error != "" {
				runErr = errors.New(message.Error)
			}
		}
	})

	close(leaseDone)

	if errors.Is(runErr, context.Canceled) || errors.Is(jobCtx.Err(), context.Canceled) {
		_ = w.controlPlane.ReportState(ctx, request.JobID, StateReport{
			AgentID: w.agentID,
			State:   "canceled",
		})
	}

	completeErr := ""
	if runErr != nil {
		completeErr = runErr.Error()
	}
	if runErr == nil && jobCtx.Err() != nil {
		completeErr = jobCtx.Err().Error()
	}
	_ = w.controlPlane.Complete(ctx, request.JobID, CompleteRequest{
		AgentID: w.agentID,
		Error:   completeErr,
	})
}

func (w *Worker) extendLease(ctx context.Context, request StepRequest, cancel context.CancelFunc, done <-chan struct{}) {
	interval := request.LeaseTTL / 2
	if interval <= 0 {
		interval = 3 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-ticker.C:
			leaseCtx, leaseCancel := context.WithTimeout(context.Background(), 5*time.Second)
			response, err := w.controlPlane.ExtendLease(leaseCtx, request.JobID, w.agentID)
			leaseCancel()
			if err != nil || response.CancelRequested {
				cancel()
				return
			}
		}
	}
}

func (w *Worker) ForwardLog(ctx context.Context, jobID string, line logstream.Line) error {
	return w.controlPlane.ReportLog(ctx, jobID, w.agentID, line)
}
