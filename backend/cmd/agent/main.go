package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/babelsuite/babelsuite/internal/agent"
	"github.com/babelsuite/babelsuite/internal/envloader"
	"github.com/babelsuite/babelsuite/internal/logstream"
	"github.com/babelsuite/babelsuite/internal/runner"
)

func main() {
	envloader.Load(".env", "../.env", "../../.env")

	agentID := firstNonEmpty(strings.TrimSpace(os.Getenv("AGENT_ID")), hostnameOr("worker"))
	agentName := firstNonEmpty(strings.TrimSpace(os.Getenv("AGENT_NAME")), "BabelSuite Worker")
	port := firstNonEmpty(strings.TrimSpace(os.Getenv("AGENT_PORT")), "8091")
	publicURL := firstNonEmpty(strings.TrimSpace(os.Getenv("AGENT_PUBLIC_URL")), "http://127.0.0.1:"+port)
	controlPlaneURL := strings.TrimSpace(os.Getenv("AGENT_CONTROL_PLANE_URL"))
	heartbeatInterval := durationOr("AGENT_HEARTBEAT_INTERVAL", 15*time.Second)
	pollInterval := durationOr("AGENT_POLL_INTERVAL", time.Second)

	localBackend := runner.NewLocal(runner.BackendConfig{
		ID:    "worker-local",
		Label: "Worker Local",
		Kind:  "local",
	})

	service := agent.NewService(agent.Info{
		AgentID:      agentID,
		Name:         agentName,
		HostURL:      publicURL,
		Status:       "ready",
		Capabilities: []string{"container", "mock", "script", "scenario"},
	}, agent.ExecutorFunc(func(ctx context.Context, request agent.StepRequest, emit func(logstream.Line)) error {
		return localBackend.Run(ctx, runner.StepSpec{
			ExecutionID:      request.ExecutionID,
			SuiteID:          request.SuiteID,
			SuiteTitle:       request.SuiteTitle,
			SuiteRepository:  request.SuiteRepository,
			Profile:          request.Profile,
			RuntimeProfile:   request.RuntimeProfile,
			Env:              cloneStringMap(request.Env),
			Headers:          cloneStringMap(request.Headers),
			SourceSuiteID:    request.SourceSuiteID,
			SourceSuiteTitle: request.SourceSuiteTitle,
			SourceRepository: request.SourceRepository,
			SourceVersion:    request.SourceVersion,
			ResolvedRef:      request.ResolvedRef,
			Digest:           request.Digest,
			DependencyAlias:  request.DependencyAlias,
			Node: runner.StepNode{
				ID:        request.Node.ID,
				Name:      request.Node.Name,
				Kind:      request.Node.Kind,
				DependsOn: append([]string{}, request.Node.DependsOn...),
			},
		}, emit)
	}))

	server := &http.Server{
		Addr:    withPortPrefix(port),
		Handler: agent.NewHandler(service),
	}

	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	controlPlane := agent.NewControlPlaneClient(controlPlaneURL, nil)
	if controlPlaneURL != "" {
		registerCtx, cancel := context.WithTimeout(rootCtx, 5*time.Second)
		if err := controlPlane.Register(registerCtx, agent.RegisterRequest{
			AgentID:      agentID,
			Name:         agentName,
			HostURL:      publicURL,
			Capabilities: []string{"container", "mock", "script", "scenario"},
		}); err != nil {
			log.Printf("agent register failed: %v", err)
		}
		cancel()

		go heartbeatLoop(rootCtx, controlPlane, agentID, heartbeatInterval)
		worker := agent.NewWorker(agentID, pollInterval, controlPlane, service)
		go func() {
			if err := worker.Run(rootCtx); err != nil && err != context.Canceled {
				log.Printf("agent worker loop stopped: %v", err)
			}
		}()
		defer func() {
			unregisterCtx, unregisterCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer unregisterCancel()
			if err := controlPlane.Unregister(unregisterCtx, agentID); err != nil {
				log.Printf("agent unregister failed: %v", err)
			}
		}()
	}

	go func() {
		<-rootCtx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	log.Printf("babelsuite agent listening on %s", server.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func heartbeatLoop(ctx context.Context, client *agent.ControlPlaneClient, agentID string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			heartbeatCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := client.Heartbeat(heartbeatCtx, agentID); err != nil {
				log.Printf("agent heartbeat failed: %v", err)
			}
			cancel()
		}
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func hostnameOr(fallback string) string {
	name, err := os.Hostname()
	if err != nil {
		return fallback
	}
	return name
}

func withPortPrefix(port string) string {
	if strings.Contains(port, ":") {
		return port
	}
	return ":" + port
}

func durationOr(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}

	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
