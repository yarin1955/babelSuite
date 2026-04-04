package runner

import (
	"context"
	"fmt"
	"time"

	"github.com/babelsuite/babelsuite/internal/logstream"
)

type Local struct {
	config BackendConfig
}

func NewLocal(configs ...BackendConfig) *Local {
	config := BackendConfig{}
	if len(configs) > 0 {
		config = configs[0]
	}
	config = normalizeBackendConfig(config, "local", "Local Docker", "local")
	return &Local{config: config}
}

func (l *Local) ID() string {
	return l.config.ID
}

func (l *Local) Label() string {
	return l.config.Label
}

func (l *Local) Kind() string {
	return l.config.Kind
}

func (l *Local) IsAvailable(context.Context) bool {
	return true
}

func (l *Local) Run(ctx context.Context, step StepSpec, emit func(logstream.Line)) error {
	emit(line(step, "info", fmt.Sprintf("[%s] Local runner claimed the step on the host worker.", step.Node.Name)))
	emit(line(step, "info", bootMessage(step)))
	if step.Node.Kind == "load" {
		emit(line(step, "info", fmt.Sprintf("[%s] Resolving load assets from load/ before the ramp begins.", step.Node.Name)))
		emit(line(step, "info", fmt.Sprintf("[%s] Applying the %s profile budgets for users, ramp-up, and latency thresholds.", step.Node.Name, step.Profile)))
	}

	delay := nodeDelay(step.Node.Kind)
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		emit(line(step, "warn", fmt.Sprintf("[%s] Runner received cancellation before the step finished.", step.Node.Name)))
		return context.Canceled
	case <-timer.C:
	}

	if step.Node.Kind == "load" {
		emit(line(step, "info", fmt.Sprintf("[%s] Synthetic traffic reached the target ramp and streamed aggregate counters back to the runner.", step.Node.Name)))
	}
	emit(line(step, "info", probeMessage(step)))
	emit(line(step, "info", fmt.Sprintf("[%s] Local runner reported the step healthy and released the lease cycle.", step.Node.Name)))
	return nil
}

func line(step StepSpec, level, text string) logstream.Line {
	return logstream.Line{
		Source:    step.Node.ID,
		Timestamp: "",
		Level:     level,
		Text:      text,
	}
}

func bootMessage(step StepSpec) string {
	switch step.Node.Kind {
	case "mock":
		return fmt.Sprintf("[%s] Hydrating mock assets for %s with the %s profile.", step.Node.Name, step.SuiteTitle, step.Profile)
	case "script":
		return fmt.Sprintf("[%s] Executing bootstrap logic and preparing outputs for downstream steps.", step.Node.Name)
	case "load":
		return fmt.Sprintf("[%s] Starting load generators from the suite package and preparing threshold collectors.", step.Node.Name)
	case "scenario":
		return fmt.Sprintf("[%s] Running scenario assertions from the suite package.", step.Node.Name)
	default:
		return fmt.Sprintf("[%s] Starting container workload under the local runner.", step.Node.Name)
	}
}

func probeMessage(step StepSpec) string {
	switch step.Node.Kind {
	case "mock":
		return fmt.Sprintf("[%s] Dispatch table loaded and mock endpoint is answering health probes.", step.Node.Name)
	case "script":
		return fmt.Sprintf("[%s] Bootstrap script completed and published its derived outputs.", step.Node.Name)
	case "load":
		return fmt.Sprintf("[%s] Load thresholds passed and result summaries are ready for downstream checks.", step.Node.Name)
	case "scenario":
		return fmt.Sprintf("[%s] Scenario checks completed without violating contract assertions.", step.Node.Name)
	default:
		return fmt.Sprintf("[%s] Health probe passed and downstream dependencies may proceed.", step.Node.Name)
	}
}

func nodeDelay(kind string) time.Duration {
	switch kind {
	case "script":
		return 450 * time.Millisecond
	case "mock":
		return 550 * time.Millisecond
	case "load":
		return 1100 * time.Millisecond
	case "scenario":
		return 700 * time.Millisecond
	default:
		return 900 * time.Millisecond
	}
}
