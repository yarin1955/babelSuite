package suites

import (
	"errors"
	"sort"
	"strings"
	"sync"
)

var ErrNotFound = errors.New("suite not found")

type ProfileOption struct {
	FileName    string `json:"fileName"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Default     bool   `json:"default"`
}

type FolderEntry struct {
	Name        string   `json:"name"`
	Role        string   `json:"role"`
	Description string   `json:"description"`
	Files       []string `json:"files"`
}

type SourceFile struct {
	Path     string `json:"path"`
	Language string `json:"language"`
	Content  string `json:"content"`
}

type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type ParameterConstraint struct {
	Name     string `json:"name"`
	Source   string `json:"source"`
	Required bool   `json:"required"`
	Forward  bool   `json:"forward"`
	Pattern  string `json:"pattern,omitempty"`
}

type MockFallback struct {
	Mode        string   `json:"mode"`
	ExampleName string   `json:"exampleName,omitempty"`
	ProxyURL    string   `json:"proxyUrl,omitempty"`
	Status      string   `json:"status,omitempty"`
	MediaType   string   `json:"mediaType,omitempty"`
	Body        string   `json:"body,omitempty"`
	Headers     []Header `json:"headers,omitempty"`
}

type MockStateTransition struct {
	OnExample           string            `json:"onExample"`
	MutationKeyTemplate string            `json:"mutationKeyTemplate,omitempty"`
	Set                 map[string]string `json:"set,omitempty"`
	Delete              []string          `json:"delete,omitempty"`
	Increment           map[string]int    `json:"increment,omitempty"`
}

type MockState struct {
	LookupKeyTemplate   string                `json:"lookupKeyTemplate,omitempty"`
	MutationKeyTemplate string                `json:"mutationKeyTemplate,omitempty"`
	Defaults            map[string]string     `json:"defaults,omitempty"`
	Transitions         []MockStateTransition `json:"transitions,omitempty"`
}

type MatchCondition struct {
	From  string `json:"from"`
	Param string `json:"param"`
	Value string `json:"value"`
}

type MockOperationMetadata struct {
	Adapter              string                `json:"adapter"`
	Dispatcher           string                `json:"dispatcher,omitempty"`
	DispatcherRules      string                `json:"dispatcherRules,omitempty"`
	DelayMillis          int                   `json:"delayMillis,omitempty"`
	ParameterConstraints []ParameterConstraint `json:"parameterConstraints,omitempty"`
	Fallback             *MockFallback         `json:"fallback,omitempty"`
	State                *MockState            `json:"state,omitempty"`
	MetadataPath         string                `json:"metadataPath,omitempty"`
	ResolverURL          string                `json:"resolverUrl,omitempty"`
	RuntimeURL           string                `json:"runtimeUrl,omitempty"`
}

type ExchangeExample struct {
	Name              string           `json:"name"`
	SourceArtifact    string           `json:"sourceArtifact"`
	When              []MatchCondition `json:"when,omitempty"`
	RequestHeaders    []Header         `json:"requestHeaders"`
	RequestBody       string           `json:"requestBody"`
	ResponseStatus    string           `json:"responseStatus"`
	ResponseMediaType string           `json:"responseMediaType"`
	ResponseHeaders   []Header         `json:"responseHeaders"`
	ResponseBody      string           `json:"responseBody"`
}

type APIOperation struct {
	ID           string                `json:"id"`
	Method       string                `json:"method"`
	Name         string                `json:"name"`
	Summary      string                `json:"summary"`
	ContractPath string                `json:"contractPath"`
	MockPath     string                `json:"mockPath"`
	MockURL      string                `json:"mockUrl"`
	CurlCommand  string                `json:"curlCommand"`
	Dispatcher   string                `json:"dispatcher,omitempty"`
	MockMetadata MockOperationMetadata `json:"mockMetadata"`
	Exchanges    []ExchangeExample     `json:"exchanges"`
}

type APISurface struct {
	ID          string         `json:"id"`
	Title       string         `json:"title"`
	Protocol    string         `json:"protocol"`
	MockHost    string         `json:"mockHost"`
	Description string         `json:"description"`
	Operations  []APIOperation `json:"operations"`
}

type Definition struct {
	ID          string          `json:"id"`
	Title       string          `json:"title"`
	Repository  string          `json:"repository"`
	Owner       string          `json:"owner"`
	Provider    string          `json:"provider"`
	Version     string          `json:"version"`
	Tags        []string        `json:"tags"`
	Description string          `json:"description"`
	Modules     []string        `json:"modules"`
	Status      string          `json:"status"`
	Score       int             `json:"score"`
	PullCommand string          `json:"pullCommand"`
	ForkCommand string          `json:"forkCommand"`
	SuiteStar   string          `json:"suiteStar"`
	Profiles    []ProfileOption `json:"profiles"`
	Folders     []FolderEntry   `json:"folders"`
	SourceFiles []SourceFile    `json:"sourceFiles"`
	Contracts   []string        `json:"contracts"`
	APISurfaces []APISurface    `json:"apiSurfaces"`
}

type Service struct {
	mu     sync.RWMutex
	suites map[string]Definition
}

func NewService() *Service {
	return &Service{
		suites: hydrateSuites(seedSuites()),
	}
}

func (s *Service) List() []Definition {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]Definition, 0, len(s.suites))
	for _, suite := range s.suites {
		result = append(result, cloneDefinition(suite))
	}

	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].Title) < strings.ToLower(result[j].Title)
	})
	return result
}

func (s *Service) Get(id string) (*Definition, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	suite, ok := s.suites[strings.TrimSpace(id)]
	if !ok {
		return nil, ErrNotFound
	}

	clone := cloneDefinition(suite)
	return &clone, nil
}

func cloneDefinition(input Definition) Definition {
	output := input
	output.Tags = append([]string{}, input.Tags...)
	output.Modules = append([]string{}, input.Modules...)
	output.Contracts = append([]string{}, input.Contracts...)
	output.Profiles = cloneProfiles(input.Profiles)
	output.Folders = cloneFolders(input.Folders)
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
	for index, file := range input {
		output[index] = file
	}
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
