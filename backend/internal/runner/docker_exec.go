package runner

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/babelsuite/babelsuite/internal/logstream"
)

var (
	dockerClientOnce sync.Once
	dockerClient     *client.Client
	dockerAvailable  bool
)

func sharedDockerClient() (*client.Client, bool) {
	dockerClientOnce.Do(func() {
		cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if err != nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if _, err := cli.Ping(ctx); err != nil {
			cli.Close()
			return
		}
		dockerClient = cli
		dockerAvailable = true
	})
	return dockerClient, dockerAvailable
}

func runInDocker(ctx context.Context, step StepSpec, emit func(logstream.Line)) error {
	cli, ok := sharedDockerClient()
	if !ok {
		return fmt.Errorf("docker daemon unavailable")
	}

	img := step.Node.Image
	if img == "" {
		return fmt.Errorf("no image configured for step %q", step.Node.Name)
	}

	emit(line(step, "info", fmt.Sprintf("[%s] Pulling image %s.", step.Node.Name, img)))
	pullOut, err := cli.ImagePull(ctx, img, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("image pull failed for %s: %w", img, err)
	}
	io.Copy(io.Discard, pullOut)
	pullOut.Close()

	env := make([]string, 0, len(step.Env))
	for k, v := range step.Env {
		env = append(env, k+"="+v)
	}

	containerName := fmt.Sprintf("babel-%s-%s", sanitizeID(step.ExecutionID), sanitizeID(step.Node.ID))
	cfg := &container.Config{
		Image: img,
		Env:   env,
		Labels: map[string]string{
			"babelsuite.execution": step.ExecutionID,
			"babelsuite.step":      step.Node.ID,
			"babelsuite.kind":      step.Node.Kind,
		},
	}
	hostCfg := &container.HostConfig{
		AutoRemove: false,
	}

	emit(line(step, "info", fmt.Sprintf("[%s] Creating container %s.", step.Node.Name, containerName)))
	created, err := cli.ContainerCreate(ctx, cfg, hostCfg, nil, nil, containerName)
	if err != nil {
		return fmt.Errorf("container create failed: %w", err)
	}
	defer func() {
		rmCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cli.ContainerRemove(rmCtx, created.ID, container.RemoveOptions{Force: true})
	}()

	if err := cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("container start failed: %w", err)
	}
	emit(line(step, "info", fmt.Sprintf("[%s] Container started.", step.Node.Name)))

	logCtx, logCancel := context.WithCancel(ctx)
	defer logCancel()
	logStream, err := cli.ContainerLogs(logCtx, created.ID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Timestamps: false,
	})
	if err == nil {
		go func() {
			defer logStream.Close()
			buf := make([]byte, 4096)
			for {
				n, err := logStream.Read(buf)
				if n > 0 {
					text := strings.TrimRight(string(buf[:n]), "\r\n")
					if len(text) > 8 {
						text = text[8:]
					}
					if text != "" {
						emit(line(step, "info", fmt.Sprintf("[%s] %s", step.Node.Name, text)))
					}
				}
				if err != nil {
					return
				}
			}
		}()
	}

	waitCh, errCh := cli.ContainerWait(ctx, created.ID, container.WaitConditionNotRunning)
	select {
	case <-ctx.Done():
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cli.ContainerStop(stopCtx, created.ID, container.StopOptions{})
		return context.Canceled
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("container wait error: %w", err)
		}
	case result := <-waitCh:
		if result.Error != nil && result.Error.Message != "" {
			return fmt.Errorf("container exited with error: %s", result.Error.Message)
		}
		if result.StatusCode != 0 {
			return fmt.Errorf("container exited with code %d", result.StatusCode)
		}
	}

	emit(line(step, "info", fmt.Sprintf("[%s] Container finished successfully.", step.Node.Name)))
	return nil
}

func resolveStepImage(step StepSpec) string {
	switch step.Node.Kind {
	case "task":
		return stepImageFromVariant(step.Node.Variant, "task")
	case "test":
		return stepImageFromVariant(step.Node.Variant, "test")
	case "service":
		return stepImageFromVariant(step.Node.Variant, "service")
	case "mock":
		return "wiremock/wiremock:3.10"
	case "traffic":
		return stepImageFromVariant(step.Node.Variant, "traffic")
	}
	return ""
}

func stepImageFromVariant(variant, _ string) string {
	switch variant {
	case "task.run", "test.run":
		return "alpine:3.19"
	case "service.wiremock":
		return "wiremock/wiremock:3.10"
	case "service.prism":
		return "stoplight/prism:5"
	case "traffic.smoke", "traffic.baseline", "traffic.stress", "traffic.spike",
		"traffic.soak", "traffic.scalability", "traffic.step", "traffic.wave",
		"traffic.staged", "traffic.constant_throughput", "traffic.constant_pacing",
		"traffic.open_model":
		return "grafana/k6:0.54"
	}
	return ""
}

func sanitizeID(id string) string {
	var b strings.Builder
	for _, ch := range id {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' {
			b.WriteRune(ch)
		} else if ch >= 'A' && ch <= 'Z' {
			b.WriteRune(ch + 32)
		} else {
			b.WriteRune('-')
		}
	}
	s := b.String()
	if len(s) > 40 {
		s = s[:40]
	}
	return strings.Trim(s, "-")
}
