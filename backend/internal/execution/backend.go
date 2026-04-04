package execution

import (
	"context"
	"strings"
	"time"

	"github.com/babelsuite/babelsuite/internal/agent"
	"github.com/babelsuite/babelsuite/internal/logstream"
	"github.com/babelsuite/babelsuite/internal/platform"
	"github.com/babelsuite/babelsuite/internal/queue"
	"github.com/babelsuite/babelsuite/internal/runner"
	"github.com/babelsuite/babelsuite/internal/suites"
)

type backendBinding struct {
	option  BackendOption
	backend runner.Backend
}

func NewServiceWithPlatform(source suiteSource, platforms platformSource, observers ...Observer) *Service {
	if source == nil {
		source = suites.NewService()
	}

	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{
		ctx:            ctx,
		cancel:         cancel,
		queue:          queue.NewMemory(ctx, 3),
		suiteSource:    source,
		platformSource: platforms,
		observers:      observers,
		logs:           logstream.NewHub(),
		suiteMeta:      seedExecutionMetadata(),
		executions:     make(map[string]*executionState),
		subs:           make(map[string]map[chan StreamEvent]struct{}),
	}
	s.signals = newTelemetrySet(s)
	s.seedHistory()
	return s
}

func (s *Service) backendOptions() []BackendOption {
	bindings := s.configuredBackends()
	options := make([]BackendOption, 0, len(bindings))
	for _, binding := range bindings {
		option := binding.option
		option.Available = s.backendAvailable(binding.backend)
		options = append(options, option)
	}
	return options
}

func (s *Service) resolveBackend(ctx context.Context, requested string) (backendBinding, error) {
	bindings := s.configuredBackends()
	if len(bindings) == 0 {
		return backendBinding{}, ErrBackendUnavailable
	}

	requested = strings.TrimSpace(requested)
	if requested != "" && !strings.EqualFold(requested, runner.AutoBackend) {
		for _, binding := range bindings {
			if binding.option.ID != requested {
				continue
			}
			if !s.backendAvailableWithContext(ctx, binding.backend) {
				return backendBinding{}, ErrBackendUnavailable
			}
			return binding, nil
		}
		return backendBinding{}, ErrBackendNotFound
	}

	for _, binding := range bindings {
		if !binding.option.Default {
			continue
		}
		if s.backendAvailableWithContext(ctx, binding.backend) {
			return binding, nil
		}
	}

	for _, binding := range bindings {
		if s.backendAvailableWithContext(ctx, binding.backend) {
			return binding, nil
		}
	}

	return backendBinding{}, ErrBackendUnavailable
}

func (s *Service) configuredBackends() []backendBinding {
	settings, err := s.loadPlatformSettings()
	if err != nil || settings == nil {
		return []backendBinding{fallbackBackendBinding()}
	}

	bindings := make([]backendBinding, 0, len(settings.Agents))
	for _, agentConfig := range settings.Agents {
		if !agentConfig.Enabled {
			continue
		}

		kind := normalizeBackendKind(agentConfig.Type)
		option := BackendOption{
			ID:          firstNonEmpty(agentConfig.AgentID, agentConfig.Name, kind),
			Label:       firstNonEmpty(agentConfig.Name, agentConfig.AgentID, kind),
			Kind:        kind,
			Description: agentConfig.Description,
			Default:     agentConfig.Default,
		}

		var backend runner.Backend
		switch kind {
		case "local":
			backend = runner.NewLocal(runner.BackendConfig{ID: option.ID, Label: option.Label, Kind: kind})
		case "kubernetes":
			backend = runner.NewKubernetes(runner.KubernetesConfig{
				BackendConfig: runner.BackendConfig{ID: option.ID, Label: option.Label, Kind: kind},
				Namespace:     agentConfig.TargetNamespace,
			})
		case "remote":
			backend = runner.NewRemote(runner.RemoteConfig{
				BackendConfig: runner.BackendConfig{ID: option.ID, Label: option.Label, Kind: kind},
				BaseURL:       agentConfig.HostURL,
				Dispatcher:    agent.NewBrokerDispatcher(option.ID, s.registry, s.coordinator),
			})
		default:
			continue
		}

		bindings = append(bindings, backendBinding{
			option:  option,
			backend: backend,
		})
	}

	if len(bindings) == 0 {
		return []backendBinding{fallbackBackendBinding()}
	}

	return bindings
}

func (s *Service) loadPlatformSettings() (*platform.PlatformSettings, error) {
	if s.platformSource == nil {
		return nil, nil
	}
	return s.platformSource.Load()
}

func (s *Service) backendAvailable(backend runner.Backend) bool {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return s.backendAvailableWithContext(ctx, backend)
}

func (s *Service) backendAvailableWithContext(ctx context.Context, backend runner.Backend) bool {
	if backend == nil {
		return false
	}
	return backend.IsAvailable(ctx)
}

func fallbackBackendBinding() backendBinding {
	option := BackendOption{
		ID:          "local-docker",
		Label:       "Local Docker",
		Kind:        "local",
		Description: "Runs on the host worker without additional remote plumbing.",
		Default:     true,
		Available:   true,
	}
	return backendBinding{
		option:  option,
		backend: runner.NewLocal(runner.BackendConfig{ID: option.ID, Label: option.Label, Kind: option.Kind}),
	}
}

func normalizeBackendKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "", "local", "docker":
		return "local"
	case "kubernetes", "k8s":
		return "kubernetes"
	case "remote", "remote-agent", "remote-docker":
		return "remote"
	default:
		return strings.ToLower(strings.TrimSpace(kind))
	}
}
