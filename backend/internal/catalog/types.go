package catalog

import (
	"errors"
	"net/http"

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

type Reader interface {
	ListPackages() ([]Package, error)
	GetPackage(id string) (*Package, error)
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
