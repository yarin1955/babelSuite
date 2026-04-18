package profiles

import (
	"errors"
	"regexp"
	"sync"
	"time"

	"github.com/babelsuite/babelsuite/internal/suites"
)

var (
	ErrSuiteNotFound   = errors.New("suite not found")
	ErrProfileNotFound = errors.New("profile not found")
)

var nonAlphaNumeric = regexp.MustCompile(`[^a-z0-9]+`)

type SecretReference struct {
	Key      string `json:"key" yaml:"key"`
	Provider string `json:"provider" yaml:"provider"`
	Ref      string `json:"ref" yaml:"ref"`
}

type Record struct {
	ID          string            `json:"id" yaml:"id"`
	Name        string            `json:"name" yaml:"name"`
	FileName    string            `json:"fileName" yaml:"fileName"`
	Description string            `json:"description" yaml:"description"`
	Scope       string            `json:"scope" yaml:"scope"`
	YAML        string            `json:"yaml" yaml:"yaml"`
	SecretRefs  []SecretReference `json:"secretRefs" yaml:"secretRefs"`
	Default     bool              `json:"default" yaml:"default"`
	ExtendsID   string            `json:"extendsId,omitempty" yaml:"extendsId,omitempty"`
	Launchable  bool              `json:"launchable" yaml:"launchable"`
	UpdatedAt   time.Time         `json:"updatedAt" yaml:"updatedAt"`
}

type SuiteSummary struct {
	ID                     string `json:"id"`
	Title                  string `json:"title"`
	Description            string `json:"description"`
	Repository             string `json:"repository"`
	ProfileCount           int    `json:"profileCount"`
	LaunchableCount        int    `json:"launchableCount"`
	DefaultProfileFileName string `json:"defaultProfileFileName"`
}

type SuiteProfiles struct {
	SuiteID                string   `json:"suiteId"`
	SuiteTitle             string   `json:"suiteTitle"`
	SuiteDescription       string   `json:"suiteDescription"`
	Repository             string   `json:"repository"`
	DefaultProfileID       string   `json:"defaultProfileId"`
	DefaultProfileFileName string   `json:"defaultProfileFileName"`
	Profiles               []Record `json:"profiles"`
}

type UpsertRequest struct {
	Name        string            `json:"name"`
	FileName    string            `json:"fileName"`
	Description string            `json:"description"`
	Scope       string            `json:"scope"`
	YAML        string            `json:"yaml"`
	SecretRefs  []SecretReference `json:"secretRefs"`
	Default     bool              `json:"default"`
	ExtendsID   string            `json:"extendsId"`
}

type suiteReader interface {
	List() []suites.Definition
	Get(id string) (*suites.Definition, error)
	Resolve(ref string) (*suites.Definition, error)
}

type Document struct {
	Suites map[string]SuiteDocument `json:"suites" yaml:"suites"`
}

type SuiteDocument struct {
	Profiles []Record `json:"profiles" yaml:"profiles"`
}

type Service struct {
	base  suiteReader
	store Store
	mu    sync.Mutex
}
