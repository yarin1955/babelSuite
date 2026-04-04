package runner

import (
	"context"
	"fmt"
	"time"

	"github.com/babelsuite/babelsuite/internal/logstream"
)

type KubernetesConfig struct {
	BackendConfig
	Namespace string
}

type Kubernetes struct {
	config    BackendConfig
	namespace string
}

func NewKubernetes(config KubernetesConfig) *Kubernetes {
	backendConfig := normalizeBackendConfig(config.BackendConfig, "kubernetes", "Kubernetes", "kubernetes")
	return &Kubernetes{
		config:    backendConfig,
		namespace: firstNonEmpty(config.Namespace, "default"),
	}
}

func (k *Kubernetes) ID() string {
	return k.config.ID
}

func (k *Kubernetes) Label() string {
	return k.config.Label
}

func (k *Kubernetes) Kind() string {
	return k.config.Kind
}

func (k *Kubernetes) IsAvailable(context.Context) bool {
	return true
}

func (k *Kubernetes) Run(ctx context.Context, step StepSpec, emit func(logstream.Line)) error {
	emit(line(step, "info", fmt.Sprintf("[%s] Scheduler reserved an execution slot in namespace %s.", step.Node.Name, k.namespace)))
	emit(line(step, "info", bootMessage(step)))

	timer := time.NewTimer(nodeDelay(step.Node.Kind) + 200*time.Millisecond)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		emit(line(step, "warn", fmt.Sprintf("[%s] Execution canceled before the workload became ready.", step.Node.Name)))
		return context.Canceled
	case <-timer.C:
	}

	emit(line(step, "info", fmt.Sprintf("[%s] Pod admitted, sidecars initialized, and readiness probes turned green.", step.Node.Name)))
	emit(line(step, "info", probeMessage(step)))
	emit(line(step, "info", fmt.Sprintf("[%s] Scheduler released the slot after the step finished cleanly.", step.Node.Name)))
	return nil
}
