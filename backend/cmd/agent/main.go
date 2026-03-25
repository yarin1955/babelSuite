package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/babelsuite/babelsuite/internal/engine"
	"github.com/babelsuite/babelsuite/internal/envloader"
	"github.com/babelsuite/babelsuite/internal/telemetry"
	"github.com/babelsuite/babelsuite/pkg/api"
	babelclient "github.com/babelsuite/babelsuite/pkg/client"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	pollInterval         = 5 * time.Second
	logBatchSize         = 50
	reportHealthInterval = 10 * time.Second
)

var errRunCanceled = errors.New("run canceled")

func main() {
	envloader.Load()

	shutdownTelemetry, err := telemetry.Setup(context.Background(), telemetry.Config{
		ServiceName:    "babelsuite-agent",
		ServiceVersion: "0.1.0",
	})
	if err != nil {
		log.Fatalf("telemetry: %v", err)
	}
	defer shutdownTelemetry(context.Background())

	serverURL := os.Getenv("SERVER_URL")
	if serverURL == "" {
		serverURL = "http://localhost:8090"
	}
	agentToken := os.Getenv("AGENT_TOKEN")
	if agentToken == "" {
		log.Fatal("AGENT_TOKEN is required")
	}
	maxWorkflows := 1
	if s := os.Getenv("MAX_WORKFLOWS"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			maxWorkflows = n
		}
	}

	server := babelclient.New(serverURL,
		babelclient.WithToken(agentToken),
		babelclient.WithHTTPClient(&http.Client{Timeout: 30 * time.Second}),
	)

	bootstrap, err := server.AgentBootstrap(context.Background())
	if err != nil {
		log.Fatalf("bootstrap: %v", err)
	}

	backendName := os.Getenv("BACKEND_ENGINE")
	if bootstrap != nil && strings.TrimSpace(bootstrap.RunnerBackend) != "" {
		backendName = strings.TrimSpace(bootstrap.RunnerBackend)
	}
	if backendName == "" {
		backendName = "docker"
	}

	targetName := os.Getenv("AGENT_TARGET_NAME")
	targetURL := os.Getenv("AGENT_TARGET_URL")
	runnerOpts := engine.RunnerOptions{}
	agentName := ""
	if bootstrap != nil {
		if bootstrap.Capacity > 0 {
			maxWorkflows = bootstrap.Capacity
		}
		agentName = strings.TrimSpace(bootstrap.AgentName)
		if bootstrap.RuntimeTarget != nil {
			targetName = bootstrap.RuntimeTarget.Name
			targetURL = bootstrap.RuntimeTarget.EndpointURL
			runnerOpts.EndpointURL = bootstrap.RuntimeTarget.EndpointURL
			runnerOpts.InsecureSkipTLSVerify = bootstrap.RuntimeTarget.InsecureSkipTLSVerify
			runnerOpts.Username = bootstrap.RuntimeTarget.Username
			runnerOpts.Password = bootstrap.RuntimeTarget.Password
			runnerOpts.BearerToken = bootstrap.RuntimeTarget.BearerToken
			runnerOpts.TLSCAData = bootstrap.RuntimeTarget.TLSCAData
			runnerOpts.TLSCertData = bootstrap.RuntimeTarget.TLSCertData
			runnerOpts.TLSKeyData = bootstrap.RuntimeTarget.TLSKeyData
			if !bootstrap.WorkerBootstrapSupported {
				reason := bootstrap.WorkerBootstrapReason
				if reason == "" {
					reason = "the assigned runtime target is not available for worker execution"
				}
				log.Fatalf("runtime target %q: %s", bootstrap.RuntimeTarget.Name, reason)
			}
		} else if !bootstrap.WorkerBootstrapSupported && bootstrap.WorkerBootstrapReason != "" {
			log.Fatalf("agent bootstrap: %s", bootstrap.WorkerBootstrapReason)
		}
	}
	if runnerOpts.EndpointURL == "" && strings.TrimSpace(targetURL) != "" {
		runnerOpts.EndpointURL = strings.TrimSpace(targetURL)
	}

	runner, err := engine.NewRunnerWithOptions(backendName, runnerOpts)
	if err != nil {
		log.Fatalf("backend: %v", err)
	}
	defer runner.Close()

	ctx := context.Background()
	if !runner.IsAvailable(ctx) {
		log.Fatalf("backend %q is not reachable", backendName)
	}
	engInfo, err := runner.Load(ctx)
	if err != nil {
		log.Fatalf("backend load: %v", err)
	}

	a := &agent{
		serverURL:    serverURL,
		name:         agentName,
		runner:       runner,
		platform:     engInfo.Platform,
		targetName:   targetName,
		targetURL:    targetURL,
		maxWorkflows: maxWorkflows,
		server:       server,
	}

	if err := a.register(context.Background()); err != nil {
		log.Printf("register: %v (continuing)", err)
	}

	log.Printf("agent started  server=%s backend=%s platform=%s workers=%d",
		serverURL, runner.Name(), engInfo.Platform, maxWorkflows)

	// Periodic health heartbeat.
	go func() {
		for {
			time.Sleep(reportHealthInterval)
			if err := a.server.AgentHealth(context.Background()); err != nil {
				log.Printf("health: %v", err)
			}
		}
	}()

	// N concurrent runners, each polling independently.
	var wg sync.WaitGroup
	for range maxWorkflows {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				run, err := a.poll(context.Background())
				if err != nil {
					log.Printf("poll: %v", err)
					time.Sleep(pollInterval)
					continue
				}
				if run == nil {
					time.Sleep(pollInterval)
					continue
				}
				log.Printf("executing run %s  image=%s", run.RunID, run.ImageRef)
				if err := a.execute(context.Background(), run); err != nil {
					log.Printf("run %s failed: %v", run.RunID, err)
					_ = a.updateRun(context.Background(), run.RunID, api.RunError)
				}
			}
		}()
	}
	wg.Wait()
}

// ── agent ─────────────────────────────────────────────────────────────────────

type agent struct {
	serverURL    string
	name         string
	runner       engine.Runner
	platform     string
	targetName   string
	targetURL    string
	maxWorkflows int
	server       *babelclient.Client
}

func (a *agent) register(ctx context.Context) error {
	ctx, span := a.startSpan(ctx, "agent.register")
	defer span.End()

	hostname, _ := os.Hostname()
	name := strings.TrimSpace(a.name)
	if name == "" {
		name = hostname
	}
	span.SetAttributes(attribute.String("agent.name", name))
	if hostname != "" {
		span.SetAttributes(attribute.String("agent.hostname", hostname))
	}
	_, err := a.server.AgentRegister(ctx, api.AgentRegisterRequest{
		Name:       name,
		Platform:   a.platform,
		Backend:    a.runner.Name(),
		TargetName: a.targetName,
		TargetURL:  a.targetURL,
		Capacity:   a.maxWorkflows,
		Version:    "0.1.0",
	})
	if err != nil {
		span.RecordError(err)
	}
	return err
}

func (a *agent) poll(ctx context.Context) (*api.Run, error) {
	ctx, span := a.startSpan(ctx, "agent.poll")
	defer span.End()

	run, err := a.server.AgentNextRun(ctx)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}
	if run != nil {
		span.SetAttributes(attribute.String("run.id", run.RunID))
	}
	return run, nil
}

func (a *agent) execute(ctx context.Context, run *api.Run) error {
	ctx, span := a.startSpan(ctx, "agent.execute_run",
		attribute.String("run.id", run.RunID),
		attribute.String("run.image_ref", run.ImageRef),
	)
	defer span.End()

	networkID, err := a.runner.CreateNetwork(ctx, "babel-net-"+run.RunID[:8])
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("network: %w", err)
	}
	defer a.runner.RemoveNetwork(ctx, networkID)

	pullStep, err := a.createStep(ctx, run.RunID, "Pull suite image")
	if err != nil {
		span.RecordError(err)
		return err
	}
	if err := a.executePull(ctx, run, pullStep, networkID); err != nil {
		span.RecordError(err)
		_ = a.finishStep(ctx, run.RunID, pullStep.StepID, api.RunFailure, 1, err.Error())
		_ = a.updateRun(ctx, run.RunID, api.RunFailure)
		return err
	}
	_ = a.finishStep(ctx, run.RunID, pullStep.StepID, api.RunSuccess, 0, "")

	runStep, err := a.createStep(ctx, run.RunID, "Run suite")
	if err != nil {
		span.RecordError(err)
		return err
	}
	exitCode, runErr := a.executeSuite(ctx, run, runStep, networkID)
	status := api.RunSuccess
	errMsg := ""
	if errors.Is(runErr, errRunCanceled) {
		status = api.RunCanceled
		errMsg = runErr.Error()
		runErr = nil
	} else if runErr != nil {
		status = api.RunError
		errMsg = runErr.Error()
	} else if exitCode != 0 {
		status = api.RunFailure
		errMsg = fmt.Sprintf("exit code %d", exitCode)
	}
	if runErr != nil {
		span.RecordError(runErr)
	}
	_ = a.finishStep(ctx, run.RunID, runStep.StepID, status, exitCode, errMsg)
	_ = a.updateRun(ctx, run.RunID, status)
	return runErr
}

func (a *agent) executePull(ctx context.Context, run *api.Run, step *api.Step, _ string) error {
	ctx, span := a.startSpan(ctx, "agent.pull_image",
		attribute.String("run.id", run.RunID),
		attribute.String("step.id", step.StepID),
		attribute.String("run.image_ref", run.ImageRef),
	)
	defer span.End()

	ch, err := a.runner.Pull(ctx, run.ImageRef)
	if err != nil {
		span.RecordError(err)
		return err
	}
	err = a.streamLines(ctx, run.RunID, step.StepID, ch)
	if err != nil {
		span.RecordError(err)
	}
	return err
}

func (a *agent) executeSuite(ctx context.Context, run *api.Run, step *api.Step, networkID string) (int, error) {
	ctx, span := a.startSpan(ctx, "agent.run_suite_container",
		attribute.String("run.id", run.RunID),
		attribute.String("step.id", step.StepID),
		attribute.String("run.image_ref", run.ImageRef),
	)
	defer span.End()

	containerID, err := a.runner.Start(ctx, engine.RunConfig{
		Name:  "babel-suite-" + run.RunID[:8],
		Image: run.ImageRef,
		Env: map[string]string{
			"BABELSUITE_RUN_ID":     run.RunID,
			"BABELSUITE_PACKAGE_ID": run.PackageID,
			"BABELSUITE_PROFILE":    run.Profile,
		},
		NetworkID: networkID,
	})
	if err != nil {
		span.RecordError(err)
		return -1, fmt.Errorf("start container: %w", err)
	}
	defer a.runner.Stop(ctx, containerID)

	waitCtx, cancelWait := context.WithCancel(ctx)
	defer cancelWait()

	var canceled atomic.Bool
	go func() {
		for {
			terminal, wasCanceled, err := a.waitForRun(waitCtx, run.RunID)
			if err != nil {
				if waitCtx.Err() != nil {
					return
				}
				log.Printf("wait run %s: %v", run.RunID, err)
				time.Sleep(2 * time.Second)
				continue
			}
			if !terminal {
				continue
			}
			if wasCanceled {
				canceled.Store(true)
			}
			a.runner.Stop(context.Background(), containerID)
			return
		}
	}()

	rc, err := a.runner.Tail(ctx, containerID)
	if err != nil {
		span.RecordError(err)
		return -1, fmt.Errorf("tail: %w", err)
	}
	defer rc.Close()

	lineNum := 0
	var batch []api.StepLogLine
	traceID, spanID := telemetry.SpanIDsFromContext(ctx)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		err := a.postLogs(ctx, run.RunID, step.StepID, batch)
		batch = batch[:0]
		return err
	}
	lw := engine.NewLineWriter(func(line []byte) {
		batch = append(batch, api.StepLogLine{
			Line:    lineNum,
			Data:    string(line),
			Time:    time.Now().UnixMilli(),
			Type:    0,
			TraceID: traceID,
			SpanID:  spanID,
		})
		lineNum++
		if len(batch) >= logBatchSize {
			_ = flush()
		}
	})
	if err := lw.WriteTo(rc); err != nil {
		span.RecordError(err)
	}
	if err := flush(); err != nil {
		span.RecordError(err)
		return -1, err
	}

	exitCode, waitErr := a.runner.Wait(ctx, containerID)
	cancelWait()
	if canceled.Load() {
		return exitCode, errRunCanceled
	}
	if waitErr != nil {
		span.RecordError(waitErr)
	}
	return exitCode, waitErr
}

func (a *agent) streamLines(ctx context.Context, runID, stepID string, ch <-chan string) error {
	var batch []api.StepLogLine
	lineNum := 0
	traceID, spanID := telemetry.SpanIDsFromContext(ctx)

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		err := a.postLogs(ctx, runID, stepID, batch)
		batch = batch[:0]
		return err
	}

	for {
		select {
		case <-ctx.Done():
			_ = flush()
			return ctx.Err()
		case line, ok := <-ch:
			if !ok {
				return flush()
			}
			batch = append(batch, api.StepLogLine{
				Line:    lineNum,
				Data:    line,
				Time:    time.Now().UnixMilli(),
				Type:    0,
				TraceID: traceID,
				SpanID:  spanID,
			})
			lineNum++
			if len(batch) >= logBatchSize {
				if err := flush(); err != nil {
					return err
				}
			}
		}
	}
}

// ── server API calls ──────────────────────────────────────────────────────────

func (a *agent) createStep(ctx context.Context, runID, name string) (*api.Step, error) {
	return a.server.AgentCreateStep(ctx, runID, api.CreateStepRequest{Name: name})
}

func (a *agent) finishStep(ctx context.Context, runID, stepID string, status api.RunStatus, exitCode int, errMsg string) error {
	_, err := a.server.AgentUpdateStep(ctx, runID, stepID, api.UpdateStepRequest{
		Status:   status,
		ExitCode: exitCode,
		Error:    errMsg,
	})
	return err
}

func (a *agent) updateRun(ctx context.Context, runID string, status api.RunStatus) error {
	_, err := a.server.AgentUpdateRun(ctx, runID, api.UpdateRunRequest{Status: status})
	return err
}

func (a *agent) postLogs(ctx context.Context, runID, stepID string, lines []api.StepLogLine) error {
	return a.server.AgentAppendLogs(ctx, runID, stepID, lines)
}

func (a *agent) waitForRun(ctx context.Context, runID string) (bool, bool, error) {
	payload, err := a.server.AgentWaitRun(ctx, runID)
	if err != nil {
		return false, false, err
	}
	if payload == nil {
		return false, false, nil
	}
	return true, payload.Canceled || payload.Status == api.RunCanceled, nil
}

func (a *agent) startSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return telemetry.Start(ctx, "babelsuite.agent", name, trace.WithAttributes(attrs...))
}

// ── http helpers ──────────────────────────────────────────────────────────────
