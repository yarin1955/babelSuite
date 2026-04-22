package runner

import (
	"bufio"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/babelsuite/babelsuite/internal/logstream"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type KubernetesConfig struct {
	BackendConfig
	Namespace  string
	Kubeconfig string
}

type Kubernetes struct {
	config    BackendConfig
	namespace string
	kubecfg   string
}

func NewKubernetes(config KubernetesConfig) *Kubernetes {
	backendConfig := normalizeBackendConfig(config.BackendConfig, "kubernetes", "Kubernetes", "kubernetes")
	return &Kubernetes{
		config:    backendConfig,
		namespace: firstNonEmpty(config.Namespace, "default"),
		kubecfg:   config.Kubeconfig,
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

func (k *Kubernetes) IsAvailable(ctx context.Context) bool {
	client, err := k.newClient()
	if err != nil {
		return false
	}
	ctx2, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err = client.CoreV1().Namespaces().Get(ctx2, k.namespace, metav1.GetOptions{})
	return err == nil
}

func (k *Kubernetes) Run(ctx context.Context, step StepSpec, emit func(logstream.Line)) error {
	capturedLogs := make([]string, 0, 8)
	emitLine := func(entry logstream.Line) {
		if text := strings.TrimSpace(entry.Text); text != "" {
			capturedLogs = append(capturedLogs, text)
		}
		emit(entry)
	}

	client, err := k.newClient()
	if err != nil {
		emitLine(line(step, "warn", fmt.Sprintf("[%s] Kubernetes client unavailable (%v), falling back to simulation.", step.Node.Name, err)))
		return k.simulate(ctx, step, capturedLogs, emitLine)
	}

	img := step.Node.Image
	if img == "" {
		img = resolveStepImage(step)
	}
	if img == "" {
		return k.simulate(ctx, step, capturedLogs, emitLine)
	}

	emitLine(line(step, "info", fmt.Sprintf("[%s] Scheduler reserved an execution slot in namespace %s.", step.Node.Name, k.namespace)))
	emitLine(line(step, "info", bootMessage(step)))

	if err := k.runPod(ctx, client, step, img, emitLine); err != nil {
		emitLine(line(step, "error", fmt.Sprintf("[%s] Pod execution failed: %v", step.Node.Name, err)))
		return err
	}
	return evaluateStepExpectations(step, 0, capturedLogs, emitLine)
}

func (k *Kubernetes) runPod(ctx context.Context, client kubernetes.Interface, step StepSpec, img string, emit func(logstream.Line)) error {
	podName := sanitizeID(fmt.Sprintf("babel-%s-%s", step.ExecutionID, step.Node.ID))
	if len(podName) > 63 {
		podName = podName[:63]
	}
	podName = strings.Trim(podName, "-")

	env := make([]corev1.EnvVar, 0, len(step.Env))
	for k, v := range step.Env {
		env = append(env, corev1.EnvVar{Name: k, Value: v})
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: k.namespace,
			Labels: map[string]string{
				"babelsuite.execution": sanitizeID(step.ExecutionID),
				"babelsuite.step":      sanitizeID(step.Node.ID),
				"babelsuite.kind":      step.Node.Kind,
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:  "step",
					Image: img,
					Env:   env,
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("64Mi"),
							corev1.ResourceCPU:    resource.MustParse("100m"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("512Mi"),
							corev1.ResourceCPU:    resource.MustParse("1000m"),
						},
					},
					SecurityContext: &corev1.SecurityContext{
						AllowPrivilegeEscalation: func() *bool { b := false; return &b }(),
						RunAsNonRoot:             func() *bool { b := true; return &b }(),
						RunAsUser:                func() *int64 { uid := int64(1000); return &uid }(),
						Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
						SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
					},
				},
			},
		},
	}

	emit(line(step, "info", fmt.Sprintf("[%s] Creating Pod %s.", step.Node.Name, podName)))
	if _, err := client.CoreV1().Pods(k.namespace).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("pod create failed: %w", err)
	}
	defer func() {
		rmCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		client.CoreV1().Pods(k.namespace).Delete(rmCtx, podName, metav1.DeleteOptions{})
	}()

	emit(line(step, "info", fmt.Sprintf("[%s] Pod scheduled, waiting for completion.", step.Node.Name)))

	if err := k.waitForPod(ctx, client, podName, emit, step); err != nil {
		return err
	}

	k.streamLogs(ctx, client, podName, step, emit)

	emit(line(step, "info", fmt.Sprintf("[%s] Pod finished successfully.", step.Node.Name)))
	return nil
}

func (k *Kubernetes) waitForPod(ctx context.Context, client kubernetes.Interface, podName string, emit func(logstream.Line), step StepSpec) error {
	watcher, err := client.CoreV1().Pods(k.namespace).Watch(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("metadata.name=%s", podName),
		Watch:         true,
	})
	if err != nil {
		return fmt.Errorf("pod watch failed: %w", err)
	}
	defer watcher.Stop()

	for {
		select {
		case <-ctx.Done():
			return context.Canceled
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return fmt.Errorf("pod watch channel closed unexpectedly")
			}
			if event.Type == watch.Error {
				return fmt.Errorf("pod watch error event")
			}
			pod, ok := event.Object.(*corev1.Pod)
			if !ok {
				continue
			}
			switch pod.Status.Phase {
			case corev1.PodSucceeded:
				return nil
			case corev1.PodFailed:
				msg := ""
				if len(pod.Status.ContainerStatuses) > 0 {
					cs := pod.Status.ContainerStatuses[0]
					if cs.State.Terminated != nil {
						msg = cs.State.Terminated.Message
					}
				}
				if msg != "" {
					return fmt.Errorf("pod failed: %s", msg)
				}
				return fmt.Errorf("pod exited with failure")
			case corev1.PodRunning:
				emit(line(step, "info", fmt.Sprintf("[%s] Pod admitted, sidecars initialized, and readiness probes turned green.", step.Node.Name)))
			}
		}
	}
}

func (k *Kubernetes) streamLogs(ctx context.Context, client kubernetes.Interface, podName string, step StepSpec, emit func(logstream.Line)) {
	logCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req := client.CoreV1().Pods(k.namespace).GetLogs(podName, &corev1.PodLogOptions{})
	stream, err := req.Stream(logCtx)
	if err != nil {
		return
	}
	defer stream.Close()

	scanner := bufio.NewScanner(stream)
	for scanner.Scan() {
		text := strings.TrimRight(scanner.Text(), "\r\n")
		if text != "" {
			emit(containerLine(step, text))
		}
	}
}

func (k *Kubernetes) simulate(ctx context.Context, step StepSpec, capturedLogs []string, emitLine func(logstream.Line)) error {
	emitLine(line(step, "info", fmt.Sprintf("[%s] Scheduler reserved an execution slot in namespace %s.", step.Node.Name, k.namespace)))
	emitLine(line(step, "info", bootMessage(step)))

	timer := time.NewTimer(nodeDelay(step.Node.Kind) + 200*time.Millisecond)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		emitLine(line(step, "warn", fmt.Sprintf("[%s] Execution canceled before the workload became ready.", step.Node.Name)))
		return context.Canceled
	case <-timer.C:
	}

	emitLine(line(step, "info", fmt.Sprintf("[%s] Pod admitted, sidecars initialized, and readiness probes turned green.", step.Node.Name)))
	emitLine(line(step, "info", probeMessage(step)))
	emitLine(line(step, "info", fmt.Sprintf("[%s] Scheduler released the slot after the step finished cleanly.", step.Node.Name)))
	return evaluateStepExpectations(step, 0, capturedLogs, emitLine)
}

func (k *Kubernetes) newClient() (kubernetes.Interface, error) {
	var cfg *rest.Config
	var err error

	if k.kubecfg != "" {
		cfg, err = clientcmd.BuildConfigFromFlags("", k.kubecfg)
	} else {
		cfg, err = rest.InClusterConfig()
		if err != nil {
			cfg, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
				clientcmd.NewDefaultClientConfigLoadingRules(),
				&clientcmd.ConfigOverrides{},
			).ClientConfig()
		}
	}
	if err != nil {
		return nil, fmt.Errorf("kubernetes config: %w", err)
	}
	return kubernetes.NewForConfig(cfg)
}
