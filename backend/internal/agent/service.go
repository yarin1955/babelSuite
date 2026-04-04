package agent

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"github.com/babelsuite/babelsuite/internal/logstream"
)

type Service struct {
	info     Info
	executor Executor

	mu   sync.Mutex
	jobs map[string]context.CancelFunc
}

func NewService(info Info, executor Executor) *Service {
	return &Service{
		info: Info{
			AgentID:      firstNonEmpty(info.AgentID, "agent-"+uuid.NewString()[:8]),
			Name:         firstNonEmpty(info.Name, "babelsuite-agent"),
			HostURL:      info.HostURL,
			Status:       firstNonEmpty(info.Status, "ready"),
			Capabilities: append([]string{}, info.Capabilities...),
		},
		executor: executor,
		jobs:     make(map[string]context.CancelFunc),
	}
}

func (s *Service) Info() Info {
	if s == nil {
		return Info{}
	}
	return s.info
}

func (s *Service) Stream(ctx context.Context, request StepRequest, emit func(StreamMessage)) {
	if s == nil || s.executor == nil {
		emit(StreamMessage{Type: "done", JobID: request.JobID, Error: "agent executor is not configured"})
		return
	}

	request.JobID = firstNonEmpty(request.JobID, request.ExecutionID+":"+request.Node.ID, uuid.NewString())
	jobCtx, cancel := context.WithCancel(ctx)
	s.registerJob(request.JobID, cancel)
	defer s.unregisterJob(request.JobID)

	emit(StreamMessage{Type: "state", JobID: request.JobID})
	err := s.executor.Run(jobCtx, request, func(line logstream.Line) {
		lineCopy := line
		emit(StreamMessage{Type: "log", JobID: request.JobID, Line: &lineCopy})
	})
	if err != nil {
		emit(StreamMessage{Type: "done", JobID: request.JobID, Error: err.Error()})
		return
	}

	emit(StreamMessage{Type: "done", JobID: request.JobID})
}

func (s *Service) Cancel(jobID string) bool {
	s.mu.Lock()
	cancel := s.jobs[jobID]
	s.mu.Unlock()
	if cancel == nil {
		return false
	}
	cancel()
	return true
}

func (s *Service) Cleanup(jobID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.jobs[jobID]; !ok {
		return false
	}
	delete(s.jobs, jobID)
	return true
}

func (s *Service) registerJob(jobID string, cancel context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[jobID] = cancel
}

func (s *Service) unregisterJob(jobID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.jobs, jobID)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
