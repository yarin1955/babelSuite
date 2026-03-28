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

type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type ExchangeExample struct {
	Name              string   `json:"name"`
	SourceArtifact    string   `json:"sourceArtifact"`
	DispatchCriteria  string   `json:"dispatchCriteria"`
	RequestHeaders    []Header `json:"requestHeaders"`
	RequestBody       string   `json:"requestBody"`
	ResponseStatus    string   `json:"responseStatus"`
	ResponseMediaType string   `json:"responseMediaType"`
	ResponseHeaders   []Header `json:"responseHeaders"`
	ResponseBody      string   `json:"responseBody"`
}

type APIOperation struct {
	ID           string            `json:"id"`
	Method       string            `json:"method"`
	Name         string            `json:"name"`
	Summary      string            `json:"summary"`
	ContractPath string            `json:"contractPath"`
	MockPath     string            `json:"mockPath"`
	MockURL      string            `json:"mockUrl"`
	CurlCommand  string            `json:"curlCommand"`
	Dispatcher   string            `json:"dispatcher"`
	Exchanges    []ExchangeExample `json:"exchanges"`
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
	Contracts   []string        `json:"contracts"`
	APISurfaces []APISurface    `json:"apiSurfaces"`
}

type Service struct {
	mu     sync.RWMutex
	suites map[string]Definition
}

func NewService() *Service {
	return &Service{
		suites: seedSuites(),
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

func cloneSurfaces(input []APISurface) []APISurface {
	output := make([]APISurface, len(input))
	for surfaceIndex, surface := range input {
		output[surfaceIndex] = surface
		output[surfaceIndex].Operations = make([]APIOperation, len(surface.Operations))
		for operationIndex, operation := range surface.Operations {
			output[surfaceIndex].Operations[operationIndex] = operation
			output[surfaceIndex].Operations[operationIndex].Exchanges = make([]ExchangeExample, len(operation.Exchanges))
			for exchangeIndex, exchange := range operation.Exchanges {
				output[surfaceIndex].Operations[operationIndex].Exchanges[exchangeIndex] = exchange
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
