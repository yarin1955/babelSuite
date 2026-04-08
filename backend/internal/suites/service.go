package suites

import (
	"sort"
	"strings"
	"sync"

	"github.com/babelsuite/babelsuite/internal/demofs"
)

type Service struct {
	mu     sync.RWMutex
	suites map[string]Definition
}

func NewService() *Service {
	return &Service{
		suites: hydrateSuites(loadDemoSuites()),
	}
}

func (s *Service) List() []Definition {
	s.mu.RLock()
	defer s.mu.RUnlock()

	catalog := suiteCatalog(s.suites)
	result := make([]Definition, 0, len(s.suites))
	for _, suite := range catalog {
		result = append(result, cloneDefinition(ResolveDefinitionTopology(suite, catalog)))
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

	catalog := suiteCatalog(s.suites)
	clone := cloneDefinition(ResolveDefinitionTopology(suite, catalog))
	return &clone, nil
}

func suiteCatalog(items map[string]Definition) []Definition {
	result := make([]Definition, 0, len(items))
	for _, suite := range items {
		result = append(result, suite)
	}
	return result
}

func loadDemoSuites() map[string]Definition {
	if !demofs.Enabled() {
		return loadWorkspaceSuites()
	}

	manifest, err := demofs.LoadManifest()
	if err != nil {
		return map[string]Definition{}
	}

	definitions, err := demofs.LoadJSON[[]Definition](manifest.SuitesFile)
	if err != nil {
		return map[string]Definition{}
	}

	result := make(map[string]Definition, len(definitions))
	for _, definition := range definitions {
		result[strings.TrimSpace(definition.ID)] = definition
	}
	return result
}
