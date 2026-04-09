package environments

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/babelsuite/babelsuite/internal/eventstream"
)

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

type systemProcessChecker struct {
	runner commandRunner
}

func (c systemProcessChecker) Alive(ctx context.Context, pid int) bool {
	if pid <= 0 {
		return false
	}

	switch runtime.GOOS {
	case "windows":
		out, err := c.runner.Run(ctx, "tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH")
		if err != nil {
			return false
		}
		text := strings.ToLower(strings.TrimSpace(string(out)))
		return text != "" && !strings.Contains(text, "no tasks are running")
	default:
		out, err := c.runner.Run(ctx, "ps", "-p", strconv.Itoa(pid), "-o", "pid=")
		if err != nil {
			return false
		}
		return strings.TrimSpace(string(out)) != ""
	}
}

func NewService() *Service {
	runner := execRunner{}
	ctx, cancel := context.WithCancel(context.Background())
	service := &Service{
		runner:  runner,
		checker: systemProcessChecker{runner: runner},
		now:     func() time.Time { return time.Now().UTC() },
		ctx:     ctx,
		cancel:  cancel,
		events:  eventstream.NewHub[streamPayload](),
	}
	service.events.Open(inventoryStreamKey)
	go service.watchInventory()
	return service
}

func newServiceForTest(runner commandRunner, checker processChecker, now func() time.Time) *Service {
	service := &Service{
		runner:  runner,
		checker: checker,
		now:     now,
		events:  eventstream.NewHub[streamPayload](),
	}
	service.events.Open(inventoryStreamKey)
	return service
}

func (s *Service) Close() {
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *Service) SubscribeEvents(ctx context.Context, since int) (<-chan StreamEvent, error) {
	if err := s.publishLatestSnapshot(ctx, "initial", false); err != nil {
		return nil, err
	}

	stream, err := s.events.Subscribe(ctx, inventoryStreamKey, since)
	if err != nil {
		return nil, err
	}

	result := make(chan StreamEvent, 32)
	go func() {
		defer close(result)
		for {
			select {
			case <-ctx.Done():
				return
			case record, ok := <-stream:
				if !ok {
					return
				}
				select {
				case result <- StreamEvent{
					ID:       record.ID,
					Reason:   record.Payload.Reason,
					Snapshot: record.Payload.Snapshot,
				}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return result, nil
}

func (s *Service) watchInventory() {
	if s.ctx == nil {
		return
	}

	_ = s.publishLatestSnapshot(s.ctx, "initial", false)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			_ = s.publishLatestSnapshot(s.ctx, "poll", true)
		}
	}
}

func (s *Service) publishLatestSnapshot(ctx context.Context, reason string, onlyOnChange bool) error {
	inventory, err := s.Snapshot(ctx)
	if err != nil {
		return err
	}

	signature := snapshotSignature(inventory)

	s.stream.mu.Lock()
	if onlyOnChange && s.stream.hasSnapshot && s.stream.lastSig == signature {
		s.stream.mu.Unlock()
		return nil
	}
	s.stream.lastSig = signature
	s.stream.hasSnapshot = true
	s.stream.mu.Unlock()

	s.events.Append(inventoryStreamKey, streamPayload{
		Reason:   reason,
		Snapshot: *inventory,
	})
	return nil
}
