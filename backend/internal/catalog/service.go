package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/babelsuite/babelsuite/internal/platform"
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
	Inspectable bool     `json:"inspectable"`
	Starred     bool     `json:"starred"`
}

type suiteReader interface {
	List() []suites.Definition
	Get(id string) (*suites.Definition, error)
}

type registrySettingsReader interface {
	Load() (*platform.PlatformSettings, error)
}

type Service struct {
	suites   suiteReader
	settings registrySettingsReader
	client   *http.Client
}

type discoveredRepository struct {
	registry platform.OCIRegistry
	name     string
	fullName string
	path     string
	tags     []string
}

type catalogIndex struct {
	packages []Package
	byID     map[string]Package
}

type knownPackageIndex struct {
	byFullName map[string]Package
	byPath     map[string]Package
}

type catalogResponse struct {
	Repositories []string `json:"repositories"`
}

type tagsResponse struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

func NewService(suites suiteReader, settings registrySettingsReader) *Service {
	return &Service{
		suites:   suites,
		settings: settings,
		client: &http.Client{
			Timeout: 4 * time.Second,
		},
	}
}

func (s *Service) ListPackages() ([]Package, error) {
	index, err := s.discover()
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
	index, err := s.discover()
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

func (s *Service) knownPackages() knownPackageIndex {
	index := knownPackageIndex{
		byFullName: make(map[string]Package, len(s.suites.List())+len(seedStdlib())),
		byPath:     make(map[string]Package, len(s.suites.List())+len(seedStdlib())),
	}

	for _, suite := range s.suites.List() {
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
			Inspectable: true,
		}
		index.add(item)
	}

	for _, item := range seedStdlib() {
		clone := item
		clone.Tags = append([]string{}, item.Tags...)
		clone.Modules = append([]string{}, item.Modules...)
		index.add(clone)
	}

	return index
}

func (s *Service) packageForRepository(repo discoveredRepository, known Package) Package {
	provider := firstNonEmpty(strings.TrimSpace(repo.registry.Provider), strings.TrimSpace(repo.registry.Name), "OCI")

	if known.ID != "" {
		item := known
		item.Repository = repo.fullName
		item.Provider = provider
		item.Tags = append([]string{}, repo.tags...)
		item.Version = chooseVersion(repo.tags, known.Version)
		item.PullCommand = buildRunCommand(repo.fullName, item.Version)
		item.ForkCommand = buildForkCommand(repo.fullName, item.Version)
		item.Inspectable = item.Kind == "suite"
		return item
	}

	kind := inferKind(repo.name)
	version := chooseVersion(repo.tags, "")
	return Package{
		ID:          packageID(repo.fullName, kind),
		Kind:        kind,
		Title:       titleForRepository(repo.name, kind),
		Repository:  repo.fullName,
		Owner:       ownerForRepository(repo.name, repo.registry.Name),
		Provider:    provider,
		Version:     version,
		Tags:        append([]string{}, repo.tags...),
		Description: genericDescription(repo.name, repo.registry.Name, kind),
		Modules:     inferModules(repo.name),
		Status:      "Verified",
		Score:       80,
		PullCommand: buildRunCommand(repo.fullName, version),
		ForkCommand: buildForkCommand(repo.fullName, version),
		Inspectable: kind == "suite",
	}
}

func (k knownPackageIndex) add(item Package) {
	fullName := normalizeRepository(item.Repository)
	k.byFullName[fullName] = item

	if repositoryPath := normalizeRepository(repositoryPath(item.Repository)); repositoryPath != "" {
		if _, exists := k.byPath[repositoryPath]; !exists {
			k.byPath[repositoryPath] = item
		}
	}
}

func (k knownPackageIndex) lookup(repo discoveredRepository) Package {
	if item, ok := k.byFullName[normalizeRepository(repo.fullName)]; ok {
		return item
	}
	if item, ok := k.byPath[normalizeRepository(repo.path)]; ok {
		return item
	}
	return Package{}
}

func normalizeRegistryURL(raw string) (string, string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", "", errors.New("registry URL is empty")
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return "", "", err
	}
	if parsed.Host == "" {
		return "", "", errors.New("registry URL host is empty")
	}

	parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	return parsed.String(), parsed.Host, nil
}

func encodeRepositoryPath(repository string) string {
	parts := strings.Split(strings.Trim(repository, "/"), "/")
	for index := range parts {
		parts[index] = url.PathEscape(parts[index])
	}
	return strings.Join(parts, "/")
}

func matchesRepositoryScope(repository, scope string) bool {
	repository = strings.Trim(strings.ToLower(repository), "/")
	scope = strings.TrimSpace(scope)
	if scope == "" || scope == "*" {
		return true
	}

	for _, candidate := range strings.Split(scope, ",") {
		candidate = strings.Trim(strings.ToLower(candidate), "/")
		if candidate == "" || candidate == "*" {
			return true
		}
		if repository == candidate || strings.HasPrefix(repository, candidate+"/") {
			return true
		}
	}

	return false
}

func normalizeRepository(repository string) string {
	return strings.ToLower(strings.TrimSpace(repository))
}

func repositoryPath(repository string) string {
	repository = strings.TrimSpace(repository)
	if repository == "" {
		return ""
	}
	if strings.Contains(repository, "://") {
		if parsed, err := url.Parse(repository); err == nil {
			return strings.Trim(parsed.Path, "/")
		}
	}
	if slash := strings.Index(repository, "/"); slash >= 0 {
		host := repository[:slash]
		if strings.Contains(host, ".") || strings.Contains(host, ":") || strings.EqualFold(host, "localhost") {
			return strings.Trim(repository[slash+1:], "/")
		}
	}
	return strings.Trim(repository, "/")
}

func chooseVersion(tags []string, preferred string) string {
	preferred = strings.TrimSpace(preferred)
	if preferred != "" {
		for _, tag := range tags {
			if tag == preferred {
				return preferred
			}
		}
	}

	for _, tag := range tags {
		if !strings.EqualFold(tag, "latest") {
			return tag
		}
	}
	if len(tags) > 0 {
		return tags[0]
	}
	if preferred != "" {
		return preferred
	}
	return "latest"
}

func sortTags(tags []string) []string {
	out := append([]string{}, tags...)
	sort.Slice(out, func(i, j int) bool {
		left := strings.ToLower(out[i])
		right := strings.ToLower(out[j])
		if left == "latest" || right == "latest" {
			return left == "latest"
		}
		return compareVersionLike(out[i], out[j]) > 0
	})
	return out
}

func compareVersionLike(left, right string) int {
	left = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(left)), "v")
	right = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(right)), "v")

	leftParts := splitVersionParts(left)
	rightParts := splitVersionParts(right)
	maxParts := len(leftParts)
	if len(rightParts) > maxParts {
		maxParts = len(rightParts)
	}

	for index := 0; index < maxParts; index++ {
		leftPart := ""
		rightPart := ""
		if index < len(leftParts) {
			leftPart = leftParts[index]
		}
		if index < len(rightParts) {
			rightPart = rightParts[index]
		}
		if leftPart == rightPart {
			continue
		}
		if isNumeric(leftPart) && isNumeric(rightPart) {
			if len(leftPart) != len(rightPart) {
				if len(leftPart) > len(rightPart) {
					return 1
				}
				return -1
			}
		}
		if leftPart > rightPart {
			return 1
		}
		return -1
	}

	return 0
}

func splitVersionParts(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		return r == '.' || r == '-' || r == '_'
	})
}

func isNumeric(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func packageID(repository, kind string) string {
	base := strings.ToLower(strings.TrimSpace(repository))
	replacer := strings.NewReplacer("/", "-", ":", "-", ".", "-", "_", "-")
	base = replacer.Replace(base)
	base = strings.Trim(base, "-")
	if base == "" {
		base = "package"
	}
	return kind + "-" + base
}

func inferKind(repository string) string {
	repository = strings.ToLower(strings.TrimSpace(repository))
	if strings.HasPrefix(repository, "babelsuite/") || strings.Contains(repository, "/babelsuite/") {
		return "stdlib"
	}
	return "suite"
}

func titleForRepository(repository, kind string) string {
	repository = strings.TrimSpace(repository)
	if kind == "stdlib" {
		name := repository
		if index := strings.Index(name, "/"); index >= 0 {
			name = name[index+1:]
		}
		return "@babelsuite/" + name
	}

	parts := strings.Split(repository, "/")
	return humanize(parts[len(parts)-1])
}

func ownerForRepository(repository, fallback string) string {
	parts := strings.Split(strings.Trim(repository, "/"), "/")
	if len(parts) == 0 {
		return firstNonEmpty(fallback, "Registry Package")
	}
	return humanize(parts[0])
}

func genericDescription(repository, registryName, kind string) string {
	if kind == "stdlib" {
		return fmt.Sprintf("Discovered in %s and treated as a BabelSuite standard library module because of its repository path.", firstNonEmpty(registryName, "the configured registry"))
	}
	return fmt.Sprintf("Discovered directly from %s. Publish richer suite metadata inside BabelSuite to unlock deep inspect, topology, and contract views.", firstNonEmpty(registryName, "the configured registry"))
}

func inferModules(repository string) []string {
	repository = strings.ToLower(strings.TrimSpace(repository))
	modules := make([]string, 0, 4)
	for _, candidate := range []string{"postgres", "kafka", "wiremock", "mock-api", "playwright", "grpc", "redis", "prometheus", "vault"} {
		if strings.Contains(repository, candidate) || (candidate == "mock-api" && strings.Contains(repository, "mock")) {
			modules = append(modules, candidate)
		}
	}
	return modules
}

func humanize(value string) string {
	parts := strings.FieldsFunc(strings.TrimSpace(value), func(r rune) bool {
		return r == '-' || r == '_' || r == '.'
	})
	if len(parts) == 0 {
		return value
	}
	for index, part := range parts {
		if part == strings.ToUpper(part) {
			parts[index] = part
			continue
		}
		parts[index] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
	}
	return strings.Join(parts, " ")
}

func buildRunCommand(repository, version string) string {
	return "babelctl run " + repository + ":" + chooseVersion(nil, version)
}

func buildForkCommand(repository, version string) string {
	name := repository
	if parts := strings.Split(strings.Trim(repository, "/"), "/"); len(parts) > 0 {
		name = parts[len(parts)-1]
	}
	return "babelctl fork " + repository + ":" + chooseVersion(nil, version) + " ./" + name
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
			Repository:  "localhost:5000/babelsuite/postgres",
			Owner:       "BabelSuite Stdlib",
			Provider:    "Stdlib",
			Version:     "1.4.0",
			Tags:        []string{"1.4.0", "1.3.2", "latest"},
			Description: "Pre-registered Starlark module for opinionated Postgres provisioning with strict connection URL contracts.",
			Modules:     []string{"typed api contract", "health checks", "auto scripts"},
			Status:      "Official",
			Score:       98,
			PullCommand: "babelctl run localhost:5000/babelsuite/postgres:1.4.0",
			ForkCommand: "babelctl fork localhost:5000/babelsuite/postgres:1.4.0 ./stdlib-postgres",
			Inspectable: false,
		},
		{
			ID:          "stdlib-kafka",
			Kind:        "stdlib",
			Title:       "@babelsuite/kafka",
			Repository:  "localhost:5000/babelsuite/kafka",
			Owner:       "BabelSuite Stdlib",
			Provider:    "Stdlib",
			Version:     "1.2.3",
			Tags:        []string{"1.2.3", "1.2.2", "latest"},
			Description: "Typed Kafka module that creates brokers, topics, and address outputs without leaking Docker wiring into suite authorship.",
			Modules:     []string{"topics", "bootstrap address", "consumer groups"},
			Status:      "Official",
			Score:       96,
			PullCommand: "babelctl run localhost:5000/babelsuite/kafka:1.2.3",
			ForkCommand: "babelctl fork localhost:5000/babelsuite/kafka:1.2.3 ./stdlib-kafka",
			Inspectable: false,
		},
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
