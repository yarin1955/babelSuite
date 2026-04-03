package suites

func cloneDefinition(input Definition) Definition {
	output := input
	output.Tags = append([]string{}, input.Tags...)
	output.Modules = append([]string{}, input.Modules...)
	output.Contracts = append([]string{}, input.Contracts...)
	output.Profiles = cloneProfiles(input.Profiles)
	output.Folders = cloneFolders(input.Folders)
	output.SeedSources = cloneSourceFiles(input.SeedSources)
	output.SourceFiles = cloneSourceFiles(input.SourceFiles)
	output.APISurfaces = cloneSurfaces(input.APISurfaces)
	return output
}

func cloneProfiles(input []ProfileOption) []ProfileOption {
	output := make([]ProfileOption, len(input))
	copy(output, input)
	return output
}

func cloneFolders(input []FolderEntry) []FolderEntry {
	output := make([]FolderEntry, len(input))
	for index, folder := range input {
		output[index] = folder
		output[index].Files = append([]string{}, folder.Files...)
	}
	return output
}

func cloneSourceFiles(input []SourceFile) []SourceFile {
	output := make([]SourceFile, len(input))
	copy(output, input)
	return output
}

func cloneSurfaces(input []APISurface) []APISurface {
	output := make([]APISurface, len(input))
	for surfaceIndex, surface := range input {
		output[surfaceIndex] = surface
		output[surfaceIndex].Operations = make([]APIOperation, len(surface.Operations))
		for operationIndex, operation := range surface.Operations {
			output[surfaceIndex].Operations[operationIndex] = operation
			output[surfaceIndex].Operations[operationIndex].MockMetadata = cloneMockMetadata(operation.MockMetadata)
			output[surfaceIndex].Operations[operationIndex].Exchanges = make([]ExchangeExample, len(operation.Exchanges))
			for exchangeIndex, exchange := range operation.Exchanges {
				output[surfaceIndex].Operations[operationIndex].Exchanges[exchangeIndex] = exchange
				output[surfaceIndex].Operations[operationIndex].Exchanges[exchangeIndex].When = append([]MatchCondition{}, exchange.When...)
				output[surfaceIndex].Operations[operationIndex].Exchanges[exchangeIndex].RequestHeaders = cloneHeaders(exchange.RequestHeaders)
				output[surfaceIndex].Operations[operationIndex].Exchanges[exchangeIndex].ResponseHeaders = cloneHeaders(exchange.ResponseHeaders)
			}
		}
	}
	return output
}

func cloneHeaders(input []Header) []Header {
	output := make([]Header, len(input))
	copy(output, input)
	return output
}

func cloneMockMetadata(input MockOperationMetadata) MockOperationMetadata {
	output := input
	output.ParameterConstraints = cloneParameterConstraints(input.ParameterConstraints)
	output.Fallback = cloneMockFallback(input.Fallback)
	output.State = cloneMockState(input.State)
	return output
}

func cloneParameterConstraints(input []ParameterConstraint) []ParameterConstraint {
	output := make([]ParameterConstraint, len(input))
	copy(output, input)
	return output
}

func cloneMockFallback(input *MockFallback) *MockFallback {
	if input == nil {
		return nil
	}

	output := *input
	output.Headers = cloneHeaders(input.Headers)
	return &output
}

func cloneMockState(input *MockState) *MockState {
	if input == nil {
		return nil
	}

	output := *input
	output.Defaults = cloneStringMap(input.Defaults)
	output.Transitions = make([]MockStateTransition, len(input.Transitions))
	for index, transition := range input.Transitions {
		output.Transitions[index] = transition
		output.Transitions[index].Set = cloneStringMap(transition.Set)
		output.Transitions[index].Delete = append([]string{}, transition.Delete...)
		output.Transitions[index].Increment = cloneIntMap(transition.Increment)
	}
	return &output
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

func cloneIntMap(input map[string]int) map[string]int {
	if len(input) == 0 {
		return nil
	}

	output := make(map[string]int, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
