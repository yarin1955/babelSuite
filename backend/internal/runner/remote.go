package runner

import (
	"context"
	"fmt"

	"github.com/babelsuite/babelsuite/internal/agent"
	"github.com/babelsuite/babelsuite/internal/logstream"
	"github.com/babelsuite/babelsuite/internal/suites"
)

type RemoteConfig struct {
	BackendConfig
	BaseURL    string
	Dispatcher agent.Dispatcher
}

type Remote struct {
	config     BackendConfig
	baseURL    string
	dispatcher agent.Dispatcher
}

func NewRemote(config RemoteConfig) *Remote {
	backendConfig := normalizeBackendConfig(config.BackendConfig, "remote", "Remote Agent", "remote")
	dispatcher := config.Dispatcher
	if dispatcher == nil {
		dispatcher = agent.NewHTTPDispatcher(config.BaseURL, nil)
	}
	return &Remote{
		config:     backendConfig,
		baseURL:    config.BaseURL,
		dispatcher: dispatcher,
	}
}

func (r *Remote) ID() string {
	return r.config.ID
}

func (r *Remote) Label() string {
	return r.config.Label
}

func (r *Remote) Kind() string {
	return r.config.Kind
}

func (r *Remote) IsAvailable(ctx context.Context) bool {
	if r.dispatcher == nil {
		return false
	}
	return r.dispatcher.IsAvailable(ctx)
}

func (r *Remote) Run(ctx context.Context, step StepSpec, emit func(logstream.Line)) error {
	if r.dispatcher == nil {
		return fmt.Errorf("remote backend %q is missing a dispatcher", r.config.ID)
	}

	emit(line(step, "info", fmt.Sprintf("[%s] Forwarding the step to the remote worker at %s.", step.Node.Name, r.baseURL)))
	return r.dispatcher.Dispatch(ctx, agent.StepRequest{
		ExecutionID:      step.ExecutionID,
		SuiteID:          step.SuiteID,
		SuiteTitle:       step.SuiteTitle,
		SuiteRepository:  step.SuiteRepository,
		Profile:          step.Profile,
		RuntimeProfile:   step.RuntimeProfile,
		Env:              cloneRunnerMap(step.Env),
		Headers:          cloneRunnerMap(step.Headers),
		Trigger:          step.Trigger,
		BackendID:        step.BackendID,
		BackendKind:      step.BackendKind,
		BackendLabel:     step.BackendLabel,
		SourceSuiteID:    step.SourceSuiteID,
		SourceSuiteTitle: step.SourceSuiteTitle,
		SourceRepository: step.SourceRepository,
		SourceVersion:    step.SourceVersion,
		ResolvedRef:      step.ResolvedRef,
		Digest:           step.Digest,
		DependencyAlias:  step.DependencyAlias,
		StepIndex:        step.StepIndex,
		TotalSteps:       step.TotalSteps,
		LeaseTTL:         step.LeaseTTL,
		Load:             cloneRunnerLoadSpec(step.Load),
		Evaluation:       cloneRunnerEvaluation(step.Evaluation),
		OnFailure:        append([]string{}, step.OnFailure...),
		ArtifactExports:  cloneRunnerArtifactExports(step.ArtifactExports),
		Node: agent.StepNode{
			ID:        step.Node.ID,
			Name:      step.Node.Name,
			Kind:      step.Node.Kind,
			Variant:   step.Node.Variant,
			DependsOn: append([]string{}, step.Node.DependsOn...),
		},
	}, emit)
}

func cloneRunnerMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}

	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func cloneRunnerArtifactExports(input []ArtifactExport) []agent.ArtifactExport {
	if len(input) == 0 {
		return nil
	}

	output := make([]agent.ArtifactExport, len(input))
	for index, item := range input {
		output[index] = agent.ArtifactExport{
			Path:   item.Path,
			Name:   item.Name,
			On:     item.On,
			Format: item.Format,
		}
	}
	return output
}

func cloneRunnerLoadSpec(input *suites.LoadSpec) *suites.LoadSpec {
	if input == nil {
		return nil
	}

	output := *input
	output.Users = make([]suites.LoadUser, len(input.Users))
	for userIndex, user := range input.Users {
		output.Users[userIndex] = user
		output.Users[userIndex].Tasks = make([]suites.LoadTask, len(user.Tasks))
		for taskIndex, task := range user.Tasks {
			output.Users[userIndex].Tasks[taskIndex] = task
			output.Users[userIndex].Tasks[taskIndex].Checks = append([]suites.LoadThreshold{}, task.Checks...)
			output.Users[userIndex].Tasks[taskIndex].Request.Headers = cloneRunnerMap(task.Request.Headers)
			output.Users[userIndex].Tasks[taskIndex].Request.Checks = append([]suites.LoadThreshold{}, task.Request.Checks...)
		}
	}
	output.Stages = append([]suites.LoadStage{}, input.Stages...)
	output.Thresholds = append([]suites.LoadThreshold{}, input.Thresholds...)
	return &output
}

func cloneRunnerEvaluation(input *suites.StepEvaluation) *suites.StepEvaluation {
	if input == nil {
		return nil
	}

	output := *input
	output.ExpectLogs = append([]string{}, input.ExpectLogs...)
	output.FailOnLogs = append([]string{}, input.FailOnLogs...)
	if input.ExpectExit != nil {
		value := *input.ExpectExit
		output.ExpectExit = &value
	}
	return &output
}
