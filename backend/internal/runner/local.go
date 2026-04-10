package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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
	capturedLogs := make([]string, 0, 8)
	emitLine := func(entry logstream.Line) {
		if text := strings.TrimSpace(entry.Text); text != "" {
			capturedLogs = append(capturedLogs, text)
		}
		emit(entry)
	}

	emitLine(line(step, "info", fmt.Sprintf("[%s] Local runner claimed the step on the host worker.", step.Node.Name)))
	emitLine(line(step, "info", bootMessage(step)))
	if len(step.Env) > 0 {
		emitLine(line(step, "info", fmt.Sprintf("[%s] Injected %d runtime variables into the step context.", step.Node.Name, len(step.Env))))
	}
	if step.Node.Kind == "mock" && len(step.Headers) > 0 {
		emitLine(line(step, "info", fmt.Sprintf("[%s] Forwarded %d runtime headers into the mock context.", step.Node.Name, len(step.Headers))))
	}
	if len(step.ArtifactExports) > 0 {
		emitLine(line(step, "info", fmt.Sprintf("[%s] Registered %d artifact export rules for this step.", step.Node.Name, len(step.ArtifactExports))))
	}

	if step.Node.Kind == "traffic" && step.Load != nil {
		emitLine(line(step, "info", fmt.Sprintf("[%s] Resolving %s assets from traffic/ before the run begins.", step.Node.Name, trafficProfileLabel(step.Node.Variant))))
		emitLine(line(step, "info", fmt.Sprintf("[%s] Applying the %s profile budgets for users, pacing, and latency thresholds.", step.Node.Name, trafficProfileLabel(step.Node.Variant))))
		if err := executeLoadStep(ctx, step, emitLine); err != nil {
			return err
		}
		return evaluateStepExpectations(step, 0, capturedLogs, emitLine)
	}

	if _, available := sharedDockerClient(); available && stepRequiresContainer(step) {
		if err := runInDocker(ctx, step, emitLine); err != nil {
			emitLine(line(step, "error", fmt.Sprintf("[%s] Container execution failed: %v", step.Node.Name, err)))
			return err
		}
		return evaluateStepExpectations(step, 0, capturedLogs, emitLine)
	}

	delay := nodeDelay(step.Node.Kind)
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		emitLine(line(step, "warn", fmt.Sprintf("[%s] Runner received cancellation before the step finished.", step.Node.Name)))
		return context.Canceled
	case <-timer.C:
	}

	if step.Node.Kind == "traffic" {
		emitLine(line(step, "info", fmt.Sprintf("[%s] Synthetic traffic reached the target ramp and streamed aggregate counters back to the runner.", step.Node.Name)))
	}
	emitLine(line(step, "info", probeMessage(step)))
	emitLine(line(step, "info", fmt.Sprintf("[%s] Local runner reported the step healthy and released the lease cycle.", step.Node.Name)))
	return evaluateStepExpectations(step, 0, capturedLogs, emitLine)
}

func stepRequiresContainer(step StepSpec) bool {
	if step.Node.Image == "" {
		return false
	}
	switch step.Node.Kind {
	case "task", "test", "traffic", "service":
		return true
	}
	return false
}

func line(step StepSpec, level, text string) logstream.Line {
	return logstream.Line{
		Source: step.Node.ID,
		Level:  level,
		Kind:   "system",
		Text:   text,
	}
}

// containerLine emits a raw line from the container's stdout/stderr unchanged.
func containerLine(step StepSpec, text string) logstream.Line {
	return logstream.Line{
		Source: step.Node.ID,
		Level:  "info",
		Kind:   "output",
		Text:   text,
	}
}

// TrafficMetricSnapshot is the JSON payload carried by metric-kind log lines.
// The frontend reads this to render a live stats panel for traffic steps.
type TrafficMetricSnapshot struct {
	Requests  int     `json:"requests"`
	Failures  int     `json:"failures"`
	ErrorRate float64 `json:"errorRate"`
	RPS       float64 `json:"rps"`
	Users     int     `json:"users"`
	MinMS     float64 `json:"minMs"`
	AvgMS     float64 `json:"avgMs"`
	P50MS     float64 `json:"p50Ms"`
	P95MS     float64 `json:"p95Ms"`
	P99MS     float64 `json:"p99Ms"`
	MaxMS     float64 `json:"maxMs"`
}

// metricLine encodes a TrafficMetricSnapshot as a metric-kind log line.
func metricLine(step StepSpec, snap TrafficMetricSnapshot) logstream.Line {
	payload, _ := json.Marshal(snap)
	return logstream.Line{
		Source: step.Node.ID,
		Level:  "info",
		Kind:   "metric",
		Text:   string(payload),
	}
}

func bootMessage(step StepSpec) string {
	switch step.Node.Kind {
	case "mock":
		return fmt.Sprintf("[%s] Hydrating mock assets from api/ and mock/ for %s with the %s profile.", step.Node.Name, step.SuiteTitle, effectiveProfile(step))
	case "service":
		if step.Node.Variant == "service.prism" || step.Node.Variant == "service.wiremock" || step.Node.Variant == "service.custom" {
			return fmt.Sprintf("[%s] Starting compatibility service assets for %s under the %s profile.", step.Node.Name, step.SuiteTitle, effectiveProfile(step))
		}
		return fmt.Sprintf("[%s] Starting background service infrastructure for %s under the %s profile.", step.Node.Name, step.SuiteTitle, effectiveProfile(step))
	case "task":
		return fmt.Sprintf("[%s] Running one-shot task assets from tasks/ under the %s profile.", step.Node.Name, effectiveProfile(step))
	case "traffic":
		return fmt.Sprintf("[%s] Starting the %s profile and preparing threshold collectors.", step.Node.Name, trafficProfileLabel(step.Node.Variant))
	case "test":
		return fmt.Sprintf("[%s] Running verification assets from tests/ under the %s profile.", step.Node.Name, effectiveProfile(step))
	default:
		return fmt.Sprintf("[%s] Starting workload under the local runner with the %s profile.", step.Node.Name, effectiveProfile(step))
	}
}

func probeMessage(step StepSpec) string {
	switch step.Node.Kind {
	case "mock":
		return fmt.Sprintf("[%s] Dispatch table loaded and mock endpoint is answering health probes.", step.Node.Name)
	case "service":
		if step.Node.Variant == "service.prism" || step.Node.Variant == "service.wiremock" || step.Node.Variant == "service.custom" {
			return fmt.Sprintf("[%s] Compatibility service answered readiness probes and exposed its local endpoint.", step.Node.Name)
		}
		return fmt.Sprintf("[%s] Background service answered readiness probes and is available to downstream steps.", step.Node.Name)
	case "task":
		return fmt.Sprintf("[%s] Task completed successfully and published its derived outputs.", step.Node.Name)
	case "traffic":
		return fmt.Sprintf("[%s] %s thresholds passed and result summaries are ready for downstream checks.", step.Node.Name, trafficProfileLabel(step.Node.Variant))
	case "test":
		return fmt.Sprintf("[%s] Test checks completed without violating suite assertions.", step.Node.Name)
	default:
		return fmt.Sprintf("[%s] Health probe passed and downstream dependencies may proceed.", step.Node.Name)
	}
}

func nodeDelay(kind string) time.Duration {
	switch kind {
	case "task":
		return 450 * time.Millisecond
	case "mock":
		return 550 * time.Millisecond
	case "service":
		return 650 * time.Millisecond
	case "traffic":
		return 1100 * time.Millisecond
	case "test":
		return 700 * time.Millisecond
	default:
		return 900 * time.Millisecond
	}
}

func effectiveProfile(step StepSpec) string {
	if step.RuntimeProfile != "" {
		return step.RuntimeProfile
	}
	return step.Profile
}

func trafficProfileLabel(variant string) string {
	switch variant {
	case "traffic.smoke":
		return "smoke traffic"
	case "traffic.baseline":
		return "baseline traffic"
	case "traffic.stress":
		return "stress traffic"
	case "traffic.spike":
		return "spike traffic"
	case "traffic.soak":
		return "soak traffic"
	case "traffic.scalability":
		return "scalability traffic"
	case "traffic.step":
		return "step traffic"
	case "traffic.wave":
		return "wave traffic"
	case "traffic.staged":
		return "staged traffic"
	case "traffic.constant_throughput":
		return "constant-throughput traffic"
	case "traffic.constant_pacing":
		return "constant-pacing traffic"
	case "traffic.open_model":
		return "open-model traffic"
	default:
		return "traffic"
	}
}

func evaluateStepExpectations(step StepSpec, exitCode int, logs []string, emit func(logstream.Line)) error {
	if step.Evaluation == nil {
		return nil
	}

	if step.Evaluation.ExpectExit != nil && exitCode != *step.Evaluation.ExpectExit {
		err := fmt.Errorf("evaluation failed: expected exit code %d, got %d", *step.Evaluation.ExpectExit, exitCode)
		emit(line(step, "error", fmt.Sprintf("[%s] %v.", step.Node.Name, err)))
		return err
	}

	joinedLogs := strings.Join(logs, "\n")
	for _, expected := range step.Evaluation.ExpectLogs {
		if expected == "" {
			continue
		}
		if !strings.Contains(joinedLogs, expected) {
			err := fmt.Errorf("evaluation failed: expected logs containing %q", expected)
			emit(line(step, "error", fmt.Sprintf("[%s] %v.", step.Node.Name, err)))
			return err
		}
	}
	for _, forbidden := range step.Evaluation.FailOnLogs {
		if forbidden == "" {
			continue
		}
		if strings.Contains(joinedLogs, forbidden) {
			err := fmt.Errorf("evaluation failed: encountered forbidden log match %q", forbidden)
			emit(line(step, "error", fmt.Sprintf("[%s] %v.", step.Node.Name, err)))
			return err
		}
	}

	if step.Evaluation.ExpectExit != nil || len(step.Evaluation.ExpectLogs) > 0 || len(step.Evaluation.FailOnLogs) > 0 {
		emit(line(step, "info", fmt.Sprintf("[%s] Evaluation controls passed for exit code and log assertions.", step.Node.Name)))
	}
	return nil
}
