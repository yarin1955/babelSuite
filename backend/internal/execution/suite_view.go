package execution

import (
	"github.com/babelsuite/babelsuite/internal/mocking"
	"github.com/babelsuite/babelsuite/internal/suites"
)

func toExecutionProfiles(input []suites.ProfileOption) []ProfileOption {
	output := make([]ProfileOption, len(input))
	for index, profile := range input {
		output[index] = ProfileOption{
			FileName:    profile.FileName,
			Label:       profile.Label,
			Description: profile.Description,
			Default:     profile.Default,
		}
	}
	return output
}

func buildExecutionSuite(suite suites.Definition) ExecutionSuite {
	renderedSurfaces := cloneExecutionSurfaces(suite.APISurfaces)
	for surfaceIndex := range renderedSurfaces {
		for operationIndex := range renderedSurfaces[surfaceIndex].Operations {
			for exchangeIndex := range renderedSurfaces[surfaceIndex].Operations[operationIndex].Exchanges {
				renderedSurfaces[surfaceIndex].Operations[operationIndex].Exchanges[exchangeIndex] = previewExchange(
					suite,
					renderedSurfaces[surfaceIndex],
					renderedSurfaces[surfaceIndex].Operations[operationIndex],
					renderedSurfaces[surfaceIndex].Operations[operationIndex].Exchanges[exchangeIndex],
				)
			}
		}
	}

	return ExecutionSuite{
		ID:                   suite.ID,
		Title:                suite.Title,
		Repository:           suite.Repository,
		SuiteStar:            suite.SuiteStar,
		Profiles:             toExecutionProfiles(suite.Profiles),
		Folders:              cloneExecutionFolders(suite.Folders),
		SourceFiles:          cloneExecutionSourceFiles(suite.SourceFiles),
		Topology:             cloneExecutionTopology(suite.Topology),
		ResolvedDependencies: cloneExecutionDependencies(suite.ResolvedDependencies),
		APISurfaces:          renderedSurfaces,
	}
}

func cloneExecutionSuite(input ExecutionSuite) ExecutionSuite {
	output := input
	output.Profiles = append([]ProfileOption{}, input.Profiles...)
	output.Folders = cloneExecutionFolders(input.Folders)
	output.SourceFiles = cloneExecutionSourceFiles(input.SourceFiles)
	output.Topology = cloneExecutionTopology(input.Topology)
	output.ResolvedDependencies = cloneExecutionDependencies(input.ResolvedDependencies)
	output.APISurfaces = cloneExecutionSurfaces(input.APISurfaces)
	return output
}

func cloneExecutionFolders(input []suites.FolderEntry) []suites.FolderEntry {
	output := make([]suites.FolderEntry, len(input))
	for index, folder := range input {
		output[index] = folder
		output[index].Files = append([]string{}, folder.Files...)
	}
	return output
}

func cloneExecutionSourceFiles(input []suites.SourceFile) []suites.SourceFile {
	output := make([]suites.SourceFile, len(input))
	copy(output, input)
	return output
}

func cloneExecutionTopology(input []suites.TopologyNode) []suites.TopologyNode {
	output := make([]suites.TopologyNode, len(input))
	for index, node := range input {
		output[index] = node
		output[index].DependsOn = append([]string{}, node.DependsOn...)
		output[index].RuntimeEnv = cloneExecutionStringMap(node.RuntimeEnv)
		output[index].RuntimeHeaders = cloneExecutionStringMap(node.RuntimeHeaders)
	}
	return output
}

func cloneExecutionDependencies(input []suites.ResolvedDependency) []suites.ResolvedDependency {
	output := make([]suites.ResolvedDependency, len(input))
	for index, dependency := range input {
		output[index] = dependency
		output[index].Inputs = cloneExecutionStringMap(dependency.Inputs)
		output[index].SourceFiles = cloneExecutionSourceFiles(dependency.SourceFiles)
	}
	return output
}

func cloneExecutionStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}

	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func cloneExecutionSurfaces(input []suites.APISurface) []suites.APISurface {
	output := make([]suites.APISurface, len(input))
	for surfaceIndex, surface := range input {
		output[surfaceIndex] = surface
		output[surfaceIndex].Operations = make([]suites.APIOperation, len(surface.Operations))
		for operationIndex, operation := range surface.Operations {
			output[surfaceIndex].Operations[operationIndex] = operation
			output[surfaceIndex].Operations[operationIndex].MockMetadata = cloneExecutionMockMetadata(operation.MockMetadata)
			output[surfaceIndex].Operations[operationIndex].Exchanges = make([]suites.ExchangeExample, len(operation.Exchanges))
			for exchangeIndex, exchange := range operation.Exchanges {
				output[surfaceIndex].Operations[operationIndex].Exchanges[exchangeIndex] = exchange
				output[surfaceIndex].Operations[operationIndex].Exchanges[exchangeIndex].When = append([]suites.MatchCondition{}, exchange.When...)
				output[surfaceIndex].Operations[operationIndex].Exchanges[exchangeIndex].RequestHeaders = append([]suites.Header{}, exchange.RequestHeaders...)
				output[surfaceIndex].Operations[operationIndex].Exchanges[exchangeIndex].ResponseHeaders = append([]suites.Header{}, exchange.ResponseHeaders...)
			}
		}
	}
	return output
}

func cloneExecutionMockMetadata(input suites.MockOperationMetadata) suites.MockOperationMetadata {
	output := input
	output.ParameterConstraints = append([]suites.ParameterConstraint{}, input.ParameterConstraints...)
	output.Fallback = cloneExecutionFallback(input.Fallback)
	output.State = cloneExecutionState(input.State)
	return output
}

func cloneExecutionFallback(input *suites.MockFallback) *suites.MockFallback {
	if input == nil {
		return nil
	}

	output := *input
	output.Headers = append([]suites.Header{}, input.Headers...)
	return &output
}

func cloneExecutionState(input *suites.MockState) *suites.MockState {
	if input == nil {
		return nil
	}

	output := *input
	if len(input.Defaults) > 0 {
		output.Defaults = make(map[string]string, len(input.Defaults))
		for key, value := range input.Defaults {
			output.Defaults[key] = value
		}
	}
	output.Transitions = make([]suites.MockStateTransition, len(input.Transitions))
	for index, transition := range input.Transitions {
		output.Transitions[index] = transition
		if len(transition.Set) > 0 {
			output.Transitions[index].Set = make(map[string]string, len(transition.Set))
			for key, value := range transition.Set {
				output.Transitions[index].Set[key] = value
			}
		}
		output.Transitions[index].Delete = append([]string{}, transition.Delete...)
		if len(transition.Increment) > 0 {
			output.Transitions[index].Increment = make(map[string]int, len(transition.Increment))
			for key, value := range transition.Increment {
				output.Transitions[index].Increment[key] = value
			}
		}
	}
	return &output
}

func previewExchange(suite suites.Definition, surface suites.APISurface, operation suites.APIOperation, exchange suites.ExchangeExample) suites.ExchangeExample {
	return mocking.PreviewExchange(suite, surface, operation, exchange)
}
