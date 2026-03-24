package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/babelsuite/babelsuite/internal/engine"
	"github.com/babelsuite/babelsuite/internal/envloader"
)

const (
	pollInterval = 5 * time.Second
	logBatchSize = 50
)

type Run struct {
	RunID    string `json:"run_id"`
	OrgID    string `json:"org_id"`
	ImageRef string `json:"image_ref"`
}

type Step struct {
	StepID string `json:"step_id"`
	RunID  string `json:"run_id"`
}

type logLine struct {
	Line int    `json:"line"`
	Data string `json:"data"`
	Time int64  `json:"time"`
	Type int    `json:"type"`
}

func main() {
	envloader.Load()

	serverURL := os.Getenv("SERVER_URL")
	if serverURL == "" {
		serverURL = "http://localhost:8090"
	}
	agentToken := os.Getenv("AGENT_TOKEN")
	if agentToken == "" {
		log.Fatal("AGENT_TOKEN is required")
	}

	docker, err := engine.NewDocker()
	if err != nil {
		log.Fatalf("docker: %v", err)
	}
	defer docker.Close()

	a := &agent{
		serverURL: serverURL,
		token:     agentToken,
		docker:    docker,
		http:      &http.Client{Timeout: 30 * time.Second},
	}

	// Register with server
	if err := a.register(); err != nil {
		log.Printf("register: %v (continuing)", err)
	}

	log.Printf("agent started  server=%s", serverURL)

	for {
		run, err := a.poll()
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
		go func(r *Run) {
			if err := a.execute(r); err != nil {
				log.Printf("run %s failed: %v", r.RunID, err)
				_ = a.updateRun(r.RunID, "error")
			}
		}(run)
	}
}

// ── agent ─────────────────────────────────────────────────────────────────────

type agent struct {
	serverURL string
	token     string
	docker    *engine.Docker
	http      *http.Client
}

func (a *agent) register() error {
	hostname, _ := os.Hostname()
	body := map[string]any{
		"name":     hostname,
		"platform": "linux",
		"backend":  "docker",
		"capacity": 1,
		"version":  "0.1.0",
	}
	_, err := a.post("/api/agent/register", body)
	return err
}

func (a *agent) poll() (*Run, error) {
	resp, err := a.get("/api/agent/runs/next")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	var run Run
	return &run, json.NewDecoder(resp.Body).Decode(&run)
}

func (a *agent) execute(run *Run) error {
	ctx := context.Background()

	networkID, err := a.docker.CreateNetwork(ctx, "babel-net-"+run.RunID[:8])
	if err != nil {
		return fmt.Errorf("network: %w", err)
	}
	defer a.docker.RemoveNetwork(ctx, networkID)

	// Step 1: pull the suite image.
	pullStep, err := a.createStep(run.RunID, "Pull suite image")
	if err != nil {
		return err
	}
	if err := a.executePull(ctx, run, pullStep, networkID); err != nil {
		_ = a.finishStep(run.RunID, pullStep.StepID, "failure", 1, err.Error())
		_ = a.updateRun(run.RunID, "failure")
		return err
	}
	_ = a.finishStep(run.RunID, pullStep.StepID, "success", 0, "")

	// Step 2: run the suite container and stream its logs.
	runStep, err := a.createStep(run.RunID, "Run suite")
	if err != nil {
		return err
	}
	exitCode, runErr := a.executeSuite(ctx, run, runStep, networkID)
	status := "success"
	errMsg := ""
	if runErr != nil {
		status = "error"
		errMsg = runErr.Error()
	} else if exitCode != 0 {
		status = "failure"
		errMsg = fmt.Sprintf("exit code %d", exitCode)
	}
	_ = a.finishStep(run.RunID, runStep.StepID, status, exitCode, errMsg)
	_ = a.updateRun(run.RunID, status)
	return runErr
}

// executePull pulls the suite OCI image and streams pull progress as log lines.
func (a *agent) executePull(ctx context.Context, run *Run, step *Step, _ string) error {
	ch, err := a.docker.Pull(ctx, run.ImageRef)
	if err != nil {
		return err
	}
	return a.streamLines(ctx, run.RunID, step.StepID, ch)
}

// executeSuite starts the suite container and streams its stdout/stderr.
// The suite OCI image is expected to be self-contained and produce output on stdout.
func (a *agent) executeSuite(ctx context.Context, run *Run, step *Step, networkID string) (int, error) {
	containerID, err := a.docker.Start(ctx, engine.RunConfig{
		Name:      "babel-suite-" + run.RunID[:8],
		Image:     run.ImageRef,
		NetworkID: networkID,
	})
	if err != nil {
		return -1, fmt.Errorf("start container: %w", err)
	}
	defer a.docker.Stop(ctx, containerID)

	// Tail logs — real stdout/stderr from the container.
	rc, err := a.docker.Tail(ctx, containerID)
	if err != nil {
		return -1, fmt.Errorf("tail: %w", err)
	}
	defer rc.Close()

	lineCh := engine.LineScanner(rc)
	if err := a.streamLines(ctx, run.RunID, step.StepID, lineCh); err != nil {
		return -1, err
	}

	return a.docker.Wait(ctx, containerID)
}

// streamLines reads lines from ch, batches them and posts to the server.
func (a *agent) streamLines(ctx context.Context, runID, stepID string, ch <-chan string) error {
	var batch []logLine
	lineNum := 0

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		err := a.postLogs(runID, stepID, batch)
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
			batch = append(batch, logLine{
				Line: lineNum,
				Data: line,
				Time: time.Now().UnixMilli(),
				Type: 0,
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

func (a *agent) createStep(runID, name string) (*Step, error) {
	resp, err := a.post(fmt.Sprintf("/api/agent/runs/%s/steps", runID), map[string]string{"name": name})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var s Step
	return &s, json.NewDecoder(resp.Body).Decode(&s)
}

func (a *agent) finishStep(runID, stepID, status string, exitCode int, errMsg string) error {
	_, err := a.patch(fmt.Sprintf("/api/agent/runs/%s/steps/%s", runID, stepID), map[string]any{
		"status":    status,
		"exit_code": exitCode,
		"error":     errMsg,
	})
	return err
}

func (a *agent) updateRun(runID, status string) error {
	_, err := a.patch(fmt.Sprintf("/api/agent/runs/%s", runID), map[string]string{"status": status})
	return err
}

func (a *agent) postLogs(runID, stepID string, lines []logLine) error {
	_, err := a.post(fmt.Sprintf("/api/agent/runs/%s/steps/%s/logs", runID, stepID), lines)
	return err
}

// ── http helpers ──────────────────────────────────────────────────────────────

func (a *agent) get(path string) (*http.Response, error) {
	req, _ := http.NewRequest(http.MethodGet, a.serverURL+path, nil)
	req.Header.Set("Authorization", "Bearer "+a.token)
	return a.http.Do(req)
}

func (a *agent) post(path string, body any) (*http.Response, error) {
	return a.do(http.MethodPost, path, body)
}

func (a *agent) patch(path string, body any) (*http.Response, error) {
	return a.do(http.MethodPatch, path, body)
}

func (a *agent) do(method, path string, body any) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, a.serverURL+path, r)
	req.Header.Set("Authorization", "Bearer "+a.token)
	req.Header.Set("Content-Type", "application/json")
	return a.http.Do(req)
}
