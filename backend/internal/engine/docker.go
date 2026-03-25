package engine

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"runtime"
	"sync"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

// Docker wraps the Docker daemon client and exposes primitives needed
// to execute suite steps and stream their logs back to the server.
// It also implements the Backend interface for multi-stage workflow execution.
type Docker struct {
	client   *client.Client
	networks sync.Map // taskUUID → network name (for Backend interface)
}

// NewDocker creates a Docker engine using the DOCKER_HOST / DOCKER_TLS*
// environment variables, falling back to the local daemon socket.
func NewDocker() (*Docker, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	return &Docker{client: cli}, nil
}

func (d *Docker) Close() error { return d.client.Close() }

func (d *Docker) Name() string { return "docker" }

func (d *Docker) IsAvailable(ctx context.Context) bool {
	_, err := d.client.Ping(ctx)
	return err == nil
}

func (d *Docker) Load(_ context.Context) (*BackendInfo, error) {
	return &BackendInfo{Platform: runtime.GOOS + "/" + runtime.GOARCH}, nil
}

// PullProgress is emitted during an image pull.
type PullProgress struct {
	Status   string `json:"status"`
	Progress string `json:"progress,omitempty"`
	ID       string `json:"id,omitempty"`
	Error    string `json:"error,omitempty"`
}

// Pull pulls an image and streams progress lines to the returned channel.
// The caller must drain the channel until it is closed.
func (d *Docker) Pull(ctx context.Context, ref string) (<-chan string, error) {
	rc, err := d.client.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return nil, fmt.Errorf("image pull %s: %w", ref, err)
	}

	ch := make(chan string, 64)
	go func() {
		defer close(ch)
		defer rc.Close()
		dec := json.NewDecoder(rc)
		for {
			var p PullProgress
			if err := dec.Decode(&p); err != nil {
				return
			}
			if p.Error != "" {
				ch <- fmt.Sprintf("error: %s", p.Error)
				return
			}
			line := p.Status
			if p.ID != "" {
				line = p.ID + ": " + line
			}
			if p.Progress != "" {
				line += "  " + p.Progress
			}
			select {
			case ch <- line:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}

// RunConfig describes a container to start.
type RunConfig struct {
	Name       string
	Image      string
	Cmd        []string
	Env        map[string]string
	NetworkID  string
	AutoRemove bool
}

// Start creates and starts a container, returning its ID.
func (d *Docker) Start(ctx context.Context, cfg RunConfig) (string, error) {
	env := make([]string, 0, len(cfg.Env))
	for k, v := range cfg.Env {
		env = append(env, k+"="+v)
	}

	containerCfg := &container.Config{
		Image:        cfg.Image,
		Cmd:          cfg.Cmd,
		Env:          env,
		AttachStdout: true,
		AttachStderr: true,
	}
	hostCfg := &container.HostConfig{
		AutoRemove: cfg.AutoRemove,
	}
	netCfg := &network.NetworkingConfig{}
	if cfg.NetworkID != "" {
		netCfg.EndpointsConfig = map[string]*network.EndpointSettings{
			cfg.NetworkID: {},
		}
	}

	resp, err := d.client.ContainerCreate(ctx, containerCfg, hostCfg, netCfg, nil, cfg.Name)
	if err != nil {
		return "", fmt.Errorf("container create %s: %w", cfg.Image, err)
	}
	if err := d.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", fmt.Errorf("container start %s: %w", resp.ID, err)
	}
	return resp.ID, nil
}

// Tail attaches to a running container and returns a reader that streams
// its combined stdout+stderr. Uses docker logs --follow under the hood.
func (d *Docker) Tail(ctx context.Context, containerID string) (io.ReadCloser, error) {
	logs, err := d.client.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	})
	if err != nil {
		return nil, fmt.Errorf("container logs %s: %w", containerID, err)
	}

	// Docker multiplexes stdout/stderr into a single stream.
	// StdCopy de-multiplexes them both into the same writer so callers
	// see a single interleaved stream, matching terminal behaviour.
	pr, pw := io.Pipe()
	go func() {
		_, err := stdcopy.StdCopy(pw, pw, logs)
		logs.Close()
		pw.CloseWithError(err)
	}()
	return pr, nil
}

// Wait blocks until the container exits and returns its exit code.
func (d *Docker) Wait(ctx context.Context, containerID string) (int, error) {
	statusCh, errCh := d.client.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		return -1, err
	case status := <-statusCh:
		return int(status.StatusCode), nil
	}
}

// Stop gracefully stops then removes a container.
func (d *Docker) Stop(ctx context.Context, containerID string) {
	timeout := 5
	_ = d.client.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout})
	_ = d.client.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true})
}

// CreateNetwork creates a bridge network and returns its ID.
func (d *Docker) CreateNetwork(ctx context.Context, name string) (string, error) {
	resp, err := d.client.NetworkCreate(ctx, name, network.CreateOptions{Driver: "bridge"})
	if err != nil {
		return "", fmt.Errorf("network create %s: %w", name, err)
	}
	return resp.ID, nil
}

// RemoveNetwork removes a network by ID.
func (d *Docker) RemoveNetwork(ctx context.Context, networkID string) {
	_ = d.client.NetworkRemove(ctx, networkID)
}

// ── Backend interface implementation ─────────────────────────────────────────
// Docker implements the Backend interface so it can be used with the Runtime
// executor for multi-stage workflow orchestration.

// SetupWorkflow creates the isolated Docker network for a workflow.
func (d *Docker) SetupWorkflow(ctx context.Context, conf *WorkflowConfig, taskUUID string) error {
	_, err := d.client.NetworkCreate(ctx, conf.Network, network.CreateOptions{Driver: "bridge"})
	if err != nil {
		return err
	}
	d.networks.Store(taskUUID, conf.Network)
	return nil
}

// StartStep creates and starts a container for the given workflow step.
// The container is connected to the workflow's network so steps can communicate.
func (d *Docker) StartStep(ctx context.Context, step *WorkflowStep, taskUUID string) error {
	env := make([]string, 0, len(step.Env))
	for k, v := range step.Env {
		env = append(env, k+"="+v)
	}

	cfg := &container.Config{
		Image:        step.Image,
		Env:          env,
		AttachStdout: true,
		AttachStderr: true,
	}
	if len(step.Entrypoint) > 0 {
		cfg.Entrypoint = step.Entrypoint
	}
	if len(step.Cmd) > 0 {
		cfg.Cmd = step.Cmd
	}

	hostCfg := &container.HostConfig{}
	netCfg := &network.NetworkingConfig{}

	// All steps in the same workflow share the workflow network so that
	// service containers (postgres etc.) are reachable by step name.
	if netName, ok := d.networks.Load(taskUUID); ok {
		netCfg.EndpointsConfig = map[string]*network.EndpointSettings{
			netName.(string): {Aliases: []string{step.Name}},
		}
	}

	containerName := stepContainerName(taskUUID, step.Name)

	// Pull explicitly if requested.
	if step.Pull {
		rc, err := d.client.ImagePull(ctx, step.Image, image.PullOptions{})
		if err == nil {
			io.Copy(io.Discard, rc) //nolint:errcheck
			rc.Close()
		}
	}

	resp, err := d.client.ContainerCreate(ctx, cfg, hostCfg, netCfg, nil, containerName)
	if err != nil {
		// Image not present locally — pull and retry.
		rc, pErr := d.client.ImagePull(ctx, step.Image, image.PullOptions{})
		if pErr != nil {
			return fmt.Errorf("pull image %s: %w", step.Image, pErr)
		}
		io.Copy(io.Discard, rc) //nolint:errcheck
		rc.Close()
		resp, err = d.client.ContainerCreate(ctx, cfg, hostCfg, netCfg, nil, containerName)
		if err != nil {
			return fmt.Errorf("create container %s: %w", containerName, err)
		}
	}

	return d.client.ContainerStart(ctx, resp.ID, container.StartOptions{})
}

// TailStep returns a reader that streams the step's combined stdout+stderr.
func (d *Docker) TailStep(ctx context.Context, step *WorkflowStep, taskUUID string) (io.ReadCloser, error) {
	logs, err := d.client.ContainerLogs(ctx, stepContainerName(taskUUID, step.Name), container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	})
	if err != nil {
		return nil, fmt.Errorf("container logs %s: %w", step.Name, err)
	}
	pr, pw := io.Pipe()
	go func() {
		_, err := stdcopy.StdCopy(pw, pw, logs)
		logs.Close()
		pw.CloseWithError(err)
	}()
	return pr, nil
}

// WaitStep blocks until the step's container exits and returns its state.
func (d *Docker) WaitStep(ctx context.Context, step *WorkflowStep, taskUUID string) (*StepState, error) {
	name := stepContainerName(taskUUID, step.Name)
	statusCh, errCh := d.client.ContainerWait(ctx, name, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		return nil, err
	case <-statusCh:
	}
	info, err := d.client.ContainerInspect(ctx, name)
	if err != nil {
		return nil, err
	}
	return &StepState{
		ExitCode:  info.State.ExitCode,
		Exited:    true,
		OOMKilled: info.State.OOMKilled,
	}, nil
}

// DestroyStep stops and removes the step's container.
func (d *Docker) DestroyStep(ctx context.Context, step *WorkflowStep, taskUUID string) error {
	name := stepContainerName(taskUUID, step.Name)
	timeout := 5
	_ = d.client.ContainerStop(ctx, name, container.StopOptions{Timeout: &timeout})
	_ = d.client.ContainerRemove(ctx, name, container.RemoveOptions{Force: true})
	return nil
}

// DestroyWorkflow removes all step containers and the workflow network.
func (d *Docker) DestroyWorkflow(ctx context.Context, conf *WorkflowConfig, taskUUID string) error {
	for _, stage := range conf.Stages {
		for _, step := range stage.Steps {
			_ = d.DestroyStep(ctx, step, taskUUID)
		}
	}
	_ = d.client.NetworkRemove(ctx, conf.Network)
	d.networks.Delete(taskUUID)
	return nil
}

// stepContainerName returns the deterministic Docker container name for a step.
func stepContainerName(taskUUID, stepName string) string {
	return "babel-" + taskUUID[:8] + "-" + stepName
}

// LineScanner wraps an io.Reader and emits one line at a time.
func LineScanner(r io.Reader) <-chan string {
	ch := make(chan string, 128)
	go func() {
		defer close(ch)
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 1024*1024), 1024*1024)
		for sc.Scan() {
			ch <- sc.Text()
		}
	}()
	return ch
}
