package catalog

import (
	"errors"
	"sort"
	"strings"

	"github.com/babelsuite/babelsuite/internal/suites"
)

var ErrNotFound = errors.New("catalog package not found")

type Package struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	Title       string   `json:"title"`
	Repository  string   `json:"repository"`
	Owner       string   `json:"owner"`
	Provider    string   `json:"provider"`
	Version     string   `json:"version"`
	Tags        []string `json:"tags"`
	Description string   `json:"description"`
	Modules     []string `json:"modules"`
	Status      string   `json:"status"`
	Score       int      `json:"score"`
	PullCommand string   `json:"pullCommand"`
	ForkCommand string   `json:"forkCommand"`
}

type suiteReader interface {
	List() []suites.Definition
	Get(id string) (*suites.Definition, error)
}

type Service struct {
	suites suiteReader
	stdlib []Package
}

func NewService(suites suiteReader) *Service {
	return &Service{
		suites: suites,
		stdlib: seedStdlib(),
	}
}

func (s *Service) ListPackages() []Package {
	suitePackages := make([]Package, 0, len(s.suites.List()))
	for _, suite := range s.suites.List() {
		suitePackages = append(suitePackages, Package{
			ID:          suite.ID,
			Kind:        "suite",
			Title:       suite.Title,
			Repository:  suite.Repository,
			Owner:       suite.Owner,
			Provider:    suite.Provider,
			Version:     suite.Version,
			Tags:        append([]string{}, suite.Tags...),
			Description: suite.Description,
			Modules:     append([]string{}, suite.Modules...),
			Status:      suite.Status,
			Score:       suite.Score,
			PullCommand: suite.PullCommand,
			ForkCommand: suite.ForkCommand,
		})
	}

	result := append(suitePackages, clonePackages(s.stdlib)...)
	sort.Slice(result, func(i, j int) bool {
		if result[i].Kind != result[j].Kind {
			return result[i].Kind < result[j].Kind
		}
		return strings.ToLower(result[i].Title) < strings.ToLower(result[j].Title)
	})
	return result
}

func (s *Service) GetPackage(id string) (*Package, error) {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return nil, ErrNotFound
	}

	if suite, err := s.suites.Get(trimmed); err == nil {
		item := Package{
			ID:          suite.ID,
			Kind:        "suite",
			Title:       suite.Title,
			Repository:  suite.Repository,
			Owner:       suite.Owner,
			Provider:    suite.Provider,
			Version:     suite.Version,
			Tags:        append([]string{}, suite.Tags...),
			Description: suite.Description,
			Modules:     append([]string{}, suite.Modules...),
			Status:      suite.Status,
			Score:       suite.Score,
			PullCommand: suite.PullCommand,
			ForkCommand: suite.ForkCommand,
		}
		return &item, nil
	}

	for _, item := range s.stdlib {
		if item.ID == trimmed {
			clone := item
			clone.Tags = append([]string{}, item.Tags...)
			clone.Modules = append([]string{}, item.Modules...)
			return &clone, nil
		}
	}

	return nil, ErrNotFound
}

func clonePackages(input []Package) []Package {
	output := make([]Package, len(input))
	for index, item := range input {
		output[index] = item
		output[index].Tags = append([]string{}, item.Tags...)
		output[index].Modules = append([]string{}, item.Modules...)
	}
	return output
}

func seedStdlib() []Package {
	return []Package{
		{
			ID:          "stdlib-postgres",
			Kind:        "stdlib",
			Title:       "@babelsuite/postgres",
			Repository:  "registry.internal/babelsuite/postgres",
			Owner:       "BabelSuite Stdlib",
			Provider:    "Stdlib",
			Version:     "1.4.0",
			Tags:        []string{"1.4.0", "1.3.2", "latest"},
			Description: "Pre-registered Starlark module for opinionated Postgres provisioning with strict connection URL contracts.",
			Modules:     []string{"typed api contract", "health checks", "auto scripts"},
			Status:      "Official",
			Score:       98,
			PullCommand: "babelctl run registry.internal/babelsuite/postgres:1.4.0",
			ForkCommand: "babelctl fork registry.internal/babelsuite/postgres:1.4.0 ./stdlib-postgres",
		},
		{
			ID:          "stdlib-kafka",
			Kind:        "stdlib",
			Title:       "@babelsuite/kafka",
			Repository:  "registry.internal/babelsuite/kafka",
			Owner:       "BabelSuite Stdlib",
			Provider:    "Stdlib",
			Version:     "1.2.3",
			Tags:        []string{"1.2.3", "1.2.2", "latest"},
			Description: "Typed Kafka module that creates brokers, topics, and address outputs without leaking Docker wiring into suite authorship.",
			Modules:     []string{"topics", "bootstrap address", "consumer groups"},
			Status:      "Official",
			Score:       96,
			PullCommand: "babelctl run registry.internal/babelsuite/kafka:1.2.3",
			ForkCommand: "babelctl fork registry.internal/babelsuite/kafka:1.2.3 ./stdlib-kafka",
		},
	}
}
