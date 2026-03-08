package engine

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

type DockerEngine struct {
	cli *client.Client
	ctx context.Context
}

type Pipeline struct {
	Name  string
	Tasks []Task
}

type Task struct {
	Name    string
	Image   string
	Command []string
}

func NewDockerEngine() (*DockerEngine, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to init docker client: %w", err)
	}

	return &DockerEngine{
		cli: cli,
		ctx: context.Background(),
	}, nil
}

func (e *DockerEngine) Execute(p *Pipeline) error {
	networkName := fmt.Sprintf("babelsuite-net-%s", p.Name)
	log.Printf("Creating ephemeral bridge network: %s", networkName)

	netResp, err := e.cli.NetworkCreate(e.ctx, networkName, network.CreateOptions{
		Driver: "bridge",
	})
	if err != nil {
		return fmt.Errorf("failed to create network: %w", err)
	}
	defer func() {
		log.Printf("Cleaning up network %s", networkName)
		e.cli.NetworkRemove(e.ctx, netResp.ID)
	}()

	for _, task := range p.Tasks {
		log.Printf("[Task: %s] Pulling image %s", task.Name, task.Image)
		out, err := e.cli.ImagePull(e.ctx, task.Image, image.PullOptions{})
		if err != nil {
			return err
		}
		io.Copy(io.Discard, out)
		out.Close()

		log.Printf("[Task: %s] Starting container (script execution)...", task.Name)

		var cmd []string
		if len(task.Command) > 0 {
			// Execute script directly: pass script to shell without compiling
			script := strings.Join(task.Command, "\n")
			cmd = []string{"/bin/sh", "-c", script}
		}

		resp, err := e.cli.ContainerCreate(e.ctx, &container.Config{
			Image:      task.Image,
			Cmd:        cmd,
			Entrypoint: []string{}, // Override default image entrypoints to execute scripts directly
			Tty:        false,
		}, &container.HostConfig{
			AutoRemove: true,
			Runtime:    "runsc", // Use gVisor (runsc) for secure container isolation
		}, &network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				networkName: {
					NetworkID: netResp.ID,
				},
			},
		}, nil, fmt.Sprintf("babelsuite-%s", task.Name))
		if err != nil {
			return err
		}

		if err := e.cli.ContainerStart(e.ctx, resp.ID, container.StartOptions{}); err != nil {
			return err
		}

		statusCh, errCh := e.cli.ContainerWait(e.ctx, resp.ID, container.WaitConditionNotRunning)
		select {
		case err := <-errCh:
			if err != nil {
				return err
			}
		case status := <-statusCh:
			log.Printf("[Task: %s] Container exited with status %d", task.Name, status.StatusCode)
		}

		outLogs, err := e.cli.ContainerLogs(e.ctx, resp.ID, container.LogsOptions{ShowStdout: true, ShowStderr: true})
		if err == nil {
			stdcopy.StdCopy(os.Stdout, os.Stderr, outLogs)
			outLogs.Close()
		}
	}

	return nil
}
