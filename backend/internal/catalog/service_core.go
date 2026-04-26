package catalog

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func NewService(suites suiteReader, settings registrySettingsReader) *Service {
	return &Service{
		suites:   suites,
		settings: settings,
		client: &http.Client{
			Timeout:   4 * time.Second,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		},
	}
}

func (s *Service) ListPackages() ([]Package, error) {
	index, err := s.discover(context.Background())
	if err != nil {
		return nil, err
	}

	packages := clonePackages(index.packages)
	sort.Slice(packages, func(i, j int) bool {
		if packages[i].Kind != packages[j].Kind {
			return packages[i].Kind < packages[j].Kind
		}
		return strings.ToLower(packages[i].Title) < strings.ToLower(packages[j].Title)
	})
	return packages, nil
}

func (s *Service) GetPackage(id string) (*Package, error) {
	index, err := s.discover(context.Background())
	if err != nil {
		return nil, err
	}

	item, ok := index.byID[strings.TrimSpace(id)]
	if !ok {
		return nil, ErrNotFound
	}

	clone := item
	clone.Tags = append([]string{}, item.Tags...)
	clone.Modules = append([]string{}, item.Modules...)
	return &clone, nil
}
