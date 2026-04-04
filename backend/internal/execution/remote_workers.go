package execution

import (
	"strings"

	"github.com/babelsuite/babelsuite/internal/agent"
	"github.com/babelsuite/babelsuite/internal/logstream"
)

func (s *Service) ConfigureRemoteWorkers(registry *agent.Registry, coordinator *agent.Coordinator) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.registry = registry
	s.coordinator = coordinator
}

func (s *Service) AssignmentState(request agent.StepRequest, state string, message string) {
	if strings.TrimSpace(message) == "" {
		switch state {
		case "claimed":
			message = "[" + request.Node.Name + "] Remote worker claimed the step and started preparing the runtime."
		case "running":
			message = "[" + request.Node.Name + "] Remote worker started executing the assigned step."
		case "canceled":
			message = "[" + request.Node.Name + "] Remote worker acknowledged the cancel request."
		default:
			message = "[" + request.Node.Name + "] Remote worker reported state " + state + "."
		}
	}

	level := "info"
	if state == "canceled" {
		level = "warn"
	}

	s.appendEvent(request.ExecutionID, ExecutionEvent{
		ID:        request.Node.ID + "-" + state,
		Source:    request.Node.ID,
		Timestamp: s.nextTimestamp(request.ExecutionID),
		Text:      message,
		Status:    "running",
		Level:     level,
	})
}

func (s *Service) AssignmentLog(request agent.StepRequest, line logstream.Line) {
	s.appendRunnerLog(request.ExecutionID, request.Node.ID, line)
}
