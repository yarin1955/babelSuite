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

// Resolve finds a suite by fuzzy-matching a raw OCI ref against suite IDs and
// repositories, using the same normalisation rules as the frontend.
func (s *Service) Resolve(ref string) (*Definition, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	normalizedRef := normalizeSuiteRef(ref)
	pathRef := repositorySuitePath(ref)
	if normalizedRef == "" {
		return nil, ErrNotFound
	}

	for _, suite := range s.suites {
		id := strings.TrimSpace(suite.ID)
		if id == normalizedRef || id == pathRef {
			catalog := suiteCatalog(s.suites)
			clone := cloneDefinition(ResolveDefinitionTopology(suite, catalog))
			return &clone, nil
		}
		suiteNorm := normalizeSuiteRef(suite.Repository)
		suitePath := repositorySuitePath(suite.Repository)
		if (suiteNorm != "" && suiteNorm == normalizedRef) || (suitePath != "" && suitePath == pathRef) {
			catalog := suiteCatalog(s.suites)
			clone := cloneDefinition(ResolveDefinitionTopology(suite, catalog))
			return &clone, nil
		}
	}
	return nil, ErrNotFound
}

// normalizeSuiteRef strips the digest and tag from a repository ref so that
// refs like "registry.io/team/suite:v1.0" compare equal to "registry.io/team/suite".
func normalizeSuiteRef(ref string) string {
	value := strings.TrimRight(strings.TrimSpace(ref), "/")
	if value == "" {
		return ""
	}
	if i := strings.Index(value, "@"); i >= 0 {
		value = value[:i]
	}
	lastSlash := strings.LastIndex(value, "/")
	lastColon := strings.LastIndex(value, ":")
	if lastColon > lastSlash {
		value = value[:lastColon]
	}
	return value
}

// repositorySuitePath strips the registry host from a normalized ref so that
// "registry.io/team/suite" and "team/suite" can match.
func repositorySuitePath(ref string) string {
	value := normalizeSuiteRef(ref)
	if value == "" {
		return ""
	}
	firstSlash := strings.Index(value, "/")
	if firstSlash < 0 {
		return value
	}
	head := value[:firstSlash]
	if head == "localhost" || strings.ContainsAny(head, ".:") {
		return value[firstSlash+1:]
	}
	return value
}

func suiteCatalog(items map[string]Definition) []Definition {
	result := make([]Definition, 0, len(items))
	for _, suite := range items {
		result = append(result, suite)
	}
	return result
}

func loadDemoSuites() map[string]Definition {
	demoSuites := loadSeedSuites()
	if demofs.Enabled() {
		return demoSuites
	}

	return mergeWorkspaceSuites(loadWorkspaceSuites(), demoSuites)
}

func loadSeedSuites() map[string]Definition {
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

func mergeWorkspaceSuites(workspace, seeded map[string]Definition) map[string]Definition {
	if len(workspace) == 0 {
		return map[string]Definition{}
	}

	result := make(map[string]Definition, len(workspace))
	for id, definition := range workspace {
		if seededDefinition, ok := seeded[id]; ok {
			definition = mergeWorkspaceDefinition(definition, seededDefinition)
		}
		result[id] = definition
	}
	return result
}

func mergeWorkspaceDefinition(workspace, seeded Definition) Definition {
	merged := workspace

	if len(merged.APISurfaces) == 0 && len(seeded.APISurfaces) > 0 {
		merged.APISurfaces = cloneSurfaces(seeded.APISurfaces)
	}
	if len(merged.Contracts) == 0 && len(seeded.Contracts) > 0 {
		merged.Contracts = append([]string{}, seeded.Contracts...)
	}
	if strings.TrimSpace(merged.Description) == "" {
		merged.Description = seeded.Description
	}
	if strings.TrimSpace(merged.Owner) == "" {
		merged.Owner = seeded.Owner
	}
	if len(merged.Labels) == 0 && len(seeded.Labels) > 0 {
		merged.Labels = cloneStringMap(seeded.Labels)
	}

	return merged
}
