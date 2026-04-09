package mocking

import (
	"errors"
	"net/http"
	"regexp"
	"sync"

	"github.com/babelsuite/babelsuite/internal/suites"
)

var (
	ErrSurfaceNotFound   = errors.New("mock surface not found")
	ErrOperationNotFound = errors.New("mock operation not found")

	templateTokenPattern = regexp.MustCompile(`(?s)\{\{\s*(.*?)\s*\}\}`)
)

type suiteReader interface {
	Get(id string) (*suites.Definition, error)
}

type Service struct {
	suites suiteReader

	mu         sync.RWMutex
	state      map[string]map[string]string
	suiteState map[string]map[string]struct{}
}

type Result struct {
	Status         int
	MediaType      string
	Headers        http.Header
	Body           []byte
	Adapter        string
	Dispatcher     string
	ResolverURL    string
	RuntimeURL     string
	MatchedExample string
}

type requestSnapshot struct {
	Method     string
	Query      map[string]string
	Headers    map[string]string
	Path       map[string]string
	BodyRaw    string
	BodyJSON   any
	BodyObject map[string]any
}

type schemaExampleDocument struct {
	Examples map[string]schemaExampleEntry `json:"examples"`
}

type schemaExampleEntry struct {
	Dispatch       []suites.MatchCondition `json:"dispatch"`
	RequestSchema  schemaRequestSpec       `json:"requestSchema"`
	ResponseSchema schemaResponseSpec      `json:"responseSchema"`
}

type schemaRequestSpec struct {
	Headers any `json:"headers"`
	Body    any `json:"body"`
}

type schemaResponseSpec struct {
	Status    string `json:"status"`
	MediaType string `json:"mediaType"`
	Headers   any    `json:"headers"`
	Body      any    `json:"body"`
}

type schemaBackedExample struct {
	Name     string
	Dispatch []suites.MatchCondition
	Request  schemaRequestSpec
	Response schemaResponseSpec
}
