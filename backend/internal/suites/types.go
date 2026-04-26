package suites

import "errors"

var (
	ErrNotFound          = errors.New("suite not found")
	ErrAlreadyExists     = errors.New("suite already exists")
	ErrUnsupportedCall   = errors.New("unsupported runtime call")
	ErrMissingDependency = errors.New("missing topology dependency")
	ErrTopologyCycle     = errors.New("topology dependency cycle")
)

type RegisterRequest struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Owner       string `json:"owner,omitempty"`
	SuiteStar   string `json:"suiteStar"`
}

const (
	NodeKindMock    = "mock"
	NodeKindService = "service"
	NodeKindTask    = "task"
	NodeKindTraffic = "traffic"
	NodeKindTest    = "test"
	NodeKindSuite   = "suite"

	VariantServiceMock     = "service.mock"
	VariantServicePrism    = "service.prism"
	VariantServiceWiremock = "service.wiremock"
	VariantServiceCustom   = "service.custom"
)

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

type SourceFile struct {
	Path     string `json:"path"`
	Language string `json:"language"`
	Content  string `json:"content"`
}

type ArtifactExport struct {
	Path   string `json:"path"`
	Name   string `json:"name,omitempty"`
	On     string `json:"on,omitempty"`
	Format string `json:"format,omitempty"`
}

type StepEvaluation struct {
	ExpectExit *int     `json:"expectExit,omitempty"`
	ExpectLogs []string `json:"expectLogs,omitempty"`
	FailOnLogs []string `json:"failOnLogs,omitempty"`
}

type TopologyNode struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	Kind              string            `json:"kind"`
	Variant           string            `json:"variant,omitempty"`
	Load              *LoadSpec         `json:"traffic,omitempty"`
	DependsOn         []string          `json:"dependsOn"`
	ResetMocks        []string          `json:"resetMocks,omitempty"`
	OnFailure         []string          `json:"onFailure,omitempty"`
	ContinueOnFailure bool              `json:"continueOnFailure,omitempty"`
	Evaluation        *StepEvaluation   `json:"evaluation,omitempty"`
	ArtifactExports   []ArtifactExport  `json:"artifactExports,omitempty"`
	Level             int               `json:"level"`
	SourceSuiteID     string            `json:"sourceSuiteId,omitempty"`
	SourceSuiteTitle  string            `json:"sourceSuiteTitle,omitempty"`
	SourceRepository  string            `json:"sourceRepository,omitempty"`
	SourceVersion     string            `json:"sourceVersion,omitempty"`
	DependencyAlias   string            `json:"dependencyAlias,omitempty"`
	ResolvedRef       string            `json:"resolvedRef,omitempty"`
	Digest            string            `json:"digest,omitempty"`
	RuntimeProfile    string            `json:"runtimeProfile,omitempty"`
	RuntimeEnv        map[string]string `json:"runtimeEnv,omitempty"`
	RuntimeHeaders    map[string]string `json:"runtimeHeaders,omitempty"`
	Order             int               `json:"-"`
}

type ResolvedDependency struct {
	Alias       string            `json:"alias"`
	Ref         string            `json:"ref"`
	Version     string            `json:"version,omitempty"`
	Resolved    string            `json:"resolved,omitempty"`
	Digest      string            `json:"digest,omitempty"`
	Profile     string            `json:"profile,omitempty"`
	Inputs      map[string]string `json:"inputs,omitempty"`
	SuiteID     string            `json:"suiteId,omitempty"`
	SuiteTitle  string            `json:"suiteTitle,omitempty"`
	Repository  string            `json:"repository,omitempty"`
	SourceFiles []SourceFile      `json:"-"`
}

type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type ParameterConstraint struct {
	Name     string `json:"name"`
	Source   string `json:"source"`
	Required bool   `json:"required"`
	Forward  bool   `json:"forward"`
	Pattern  string `json:"pattern,omitempty"`
}

type MockFallback struct {
	Mode        string   `json:"mode"`
	ExampleName string   `json:"exampleName,omitempty"`
	ProxyURL    string   `json:"proxyUrl,omitempty"`
	Status      string   `json:"status,omitempty"`
	MediaType   string   `json:"mediaType,omitempty"`
	Body        string   `json:"body,omitempty"`
	Headers     []Header `json:"headers,omitempty"`
}

type MockStateTransition struct {
	OnExample           string            `json:"onExample"`
	MutationKeyTemplate string            `json:"mutationKeyTemplate,omitempty"`
	Set                 map[string]string `json:"set,omitempty"`
	Delete              []string          `json:"delete,omitempty"`
	Increment           map[string]int    `json:"increment,omitempty"`
}

type MockState struct {
	LookupKeyTemplate   string                `json:"lookupKeyTemplate,omitempty"`
	MutationKeyTemplate string                `json:"mutationKeyTemplate,omitempty"`
	Defaults            map[string]string     `json:"defaults,omitempty"`
	Transitions         []MockStateTransition `json:"transitions,omitempty"`
}

type MatchCondition struct {
	From  string `json:"from"`
	Param string `json:"param"`
	Value string `json:"value"`
}

type MockOperationMetadata struct {
	Adapter              string                `json:"adapter"`
	Dispatcher           string                `json:"dispatcher,omitempty"`
	DispatcherRules      string                `json:"dispatcherRules,omitempty"`
	DelayMillis          int                   `json:"delayMillis,omitempty"`
	ParameterConstraints []ParameterConstraint `json:"parameterConstraints,omitempty"`
	Fallback             *MockFallback         `json:"fallback,omitempty"`
	State                *MockState            `json:"state,omitempty"`
	MetadataPath         string                `json:"metadataPath,omitempty"`
	ResolverURL          string                `json:"resolverUrl,omitempty"`
	RuntimeURL           string                `json:"runtimeUrl,omitempty"`
}

type ExchangeExample struct {
	Name              string           `json:"name"`
	SourceArtifact    string           `json:"sourceArtifact"`
	When              []MatchCondition `json:"when,omitempty"`
	RequestHeaders    []Header         `json:"requestHeaders"`
	RequestBody       string           `json:"requestBody"`
	ResponseStatus    string           `json:"responseStatus"`
	ResponseMediaType string           `json:"responseMediaType"`
	ResponseHeaders   []Header         `json:"responseHeaders"`
	ResponseBody      string           `json:"responseBody"`
}

type APIOperation struct {
	ID           string                `json:"id"`
	Method       string                `json:"method"`
	Name         string                `json:"name"`
	Summary      string                `json:"summary"`
	ContractPath string                `json:"contractPath"`
	MockPath     string                `json:"mockPath"`
	MockURL      string                `json:"mockUrl"`
	CurlCommand  string                `json:"curlCommand"`
	Dispatcher   string                `json:"dispatcher,omitempty"`
	MockMetadata MockOperationMetadata `json:"mockMetadata"`
	Exchanges    []ExchangeExample     `json:"exchanges"`
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
	ID                   string               `json:"id"`
	Title                string               `json:"title"`
	Repository           string               `json:"repository"`
	Owner                string               `json:"owner"`
	Provider             string               `json:"provider"`
	Version              string               `json:"version"`
	Labels               map[string]string    `json:"labels,omitempty"`
	Tags                 []string             `json:"tags"`
	Description          string               `json:"description"`
	Modules              []string             `json:"modules"`
	Status               string               `json:"status"`
	Score                int                  `json:"score"`
	PullCommand          string               `json:"pullCommand"`
	ForkCommand          string               `json:"forkCommand"`
	SuiteStar            string               `json:"suiteStar"`
	Profiles             []ProfileOption      `json:"profiles"`
	Folders              []FolderEntry        `json:"folders"`
	SeedSources          []SourceFile         `json:"-"`
	SourceFiles          []SourceFile         `json:"sourceFiles"`
	Topology             []TopologyNode       `json:"topology,omitempty"`
	TopologyError        string               `json:"topologyError,omitempty"`
	ResolvedDependencies []ResolvedDependency `json:"resolvedDependencies,omitempty"`
	Contracts            []string             `json:"contracts"`
	APISurfaces          []APISurface         `json:"apiSurfaces"`
}
