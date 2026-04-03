package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/babelsuite/babelsuite/internal/platform"
)

func (s *Service) discover() (*catalogIndex, error) {
	settings, err := s.settings.Load()
	if err != nil {
		return nil, err
	}

	knownPackages := s.knownPackages()
	packages := make([]Package, 0, len(knownPackages.byFullName))
	byID := make(map[string]Package)
	seenRepositories := make(map[string]struct{})
	failures := make([]string, 0)
	successes := 0

	for _, registry := range settings.Registries {
		discovered, err := s.discoverRegistry(registry)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", firstNonEmpty(registry.Name, registry.RegistryID, registry.RegistryURL), err))
			continue
		}

		successes++
		for _, repo := range discovered {
			key := normalizeRepository(repo.fullName)
			if _, exists := seenRepositories[key]; exists {
				continue
			}
			seenRepositories[key] = struct{}{}

			item := s.packageForRepository(repo, knownPackages.lookup(repo))
			packages = append(packages, item)
			byID[item.ID] = item
		}
	}

	if successes == 0 && len(failures) > 0 {
		return nil, errors.New(strings.Join(failures, "; "))
	}

	return &catalogIndex{packages: packages, byID: byID}, nil
}

func (s *Service) discoverRegistry(registry platform.OCIRegistry) ([]discoveredRepository, error) {
	baseURL, host, err := normalizeRegistryURL(registry.RegistryURL)
	if err != nil {
		return nil, err
	}

	catalogURL := strings.TrimRight(baseURL, "/") + "/v2/_catalog?n=1000"
	var catalog catalogResponse
	if err := s.getJSON(catalogURL, registry, &catalog); err != nil {
		return nil, err
	}

	discovered := make([]discoveredRepository, 0, len(catalog.Repositories))
	for _, repository := range catalog.Repositories {
		repository = strings.TrimSpace(repository)
		if repository == "" || !matchesRepositoryScope(repository, registry.RepositoryScope) {
			continue
		}

		tagsURL := strings.TrimRight(baseURL, "/") + "/v2/" + encodeRepositoryPath(repository) + "/tags/list"
		var tags tagsResponse
		if err := s.getJSON(tagsURL, registry, &tags); err != nil {
			continue
		}
		if len(tags.Tags) == 0 {
			continue
		}

		discovered = append(discovered, discoveredRepository{
			registry: registry,
			name:     repository,
			fullName: host + "/" + repository,
			path:     repository,
			tags:     sortTags(tags.Tags),
		})
	}

	return discovered, nil
}

func (s *Service) getJSON(target string, registry platform.OCIRegistry, out any) error {
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return err
	}

	username := strings.TrimSpace(registry.Username)
	secret := strings.TrimSpace(registry.Secret)
	if strings.Contains(secret, "://") {
		secret = ""
	}
	if username != "" && secret != "" {
		req.SetBasicAuth(username, secret)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("registry returned %s", resp.Status)
	}

	return json.NewDecoder(resp.Body).Decode(out)
}
