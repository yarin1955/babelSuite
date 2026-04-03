package apisix

import "regexp"

const (
	engineAddrEnv        = "${{BABELSUITE_ENGINE_ADDR}}"
	grpcUpstreamAddrEnv  = "${{APISIX_GRPC_UPSTREAM_ADDR}}"
	kafkaUpstreamAddrEnv = "${{APISIX_KAFKA_UPSTREAM_ADDR}}"
	mqttUpstreamAddrEnv  = "${{APISIX_MQTT_UPSTREAM_ADDR}}"
	tcpUpstreamAddrEnv   = "${{APISIX_TCP_UPSTREAM_ADDR}}"
	udpUpstreamAddrEnv   = "${{APISIX_UDP_UPSTREAM_ADDR}}"
)

var pathParameterPattern = regexp.MustCompile(`\{[^/]+\}`)

type SuiteConfig struct {
	ID          string
	APISurfaces []SurfaceConfig
}

type SurfaceConfig struct {
	ID         string
	Protocol   string
	MockHost   string
	Operations []OperationConfig
}

type OperationConfig struct {
	ID              string
	Method          string
	Name            string
	Summary         string
	ContractPath    string
	ContractContent string
	MockURL         string
	MockMetadata    OperationMetadataConfig
}

type OperationMetadataConfig struct {
	Adapter         string
	DispatcherRules string
	ResolverURL     string
	RuntimeURL      string
}

type routeDocument struct {
	Deployment   deploymentBlock      `yaml:"deployment"`
	Plugins      []pluginSpec         `yaml:"plugins,omitempty"`
	Protos       []protoBlock         `yaml:"protos,omitempty"`
	Upstreams    []namedUpstreamBlock `yaml:"upstreams,omitempty"`
	Routes       []routeBlock         `yaml:"routes,omitempty"`
	StreamRoutes []streamRouteBlock   `yaml:"stream_routes,omitempty"`
}

type pluginSpec struct {
	Name   string `yaml:"name"`
	Stream bool   `yaml:"stream,omitempty"`
}

type deploymentBlock struct {
	Role          string              `yaml:"role"`
	RoleDataPlane deploymentDataPlane `yaml:"role_data_plane"`
}

type deploymentDataPlane struct {
	ConfigProvider string `yaml:"config_provider"`
}

type routeBlock struct {
	ID              string         `yaml:"id"`
	Name            string         `yaml:"name,omitempty"`
	Desc            string         `yaml:"desc,omitempty"`
	URI             string         `yaml:"uri"`
	Methods         []string       `yaml:"methods,omitempty"`
	Hosts           []string       `yaml:"hosts,omitempty"`
	EnableWebsocket bool           `yaml:"enable_websocket,omitempty"`
	Plugins         map[string]any `yaml:"plugins,omitempty"`
	Upstream        upstreamBlock  `yaml:"upstream"`
}

type upstreamBlock struct {
	Type   string         `yaml:"type"`
	Scheme string         `yaml:"scheme,omitempty"`
	Nodes  map[string]int `yaml:"nodes"`
}

type namedUpstreamBlock struct {
	ID     string         `yaml:"id"`
	Type   string         `yaml:"type"`
	Scheme string         `yaml:"scheme,omitempty"`
	Nodes  map[string]int `yaml:"nodes"`
}

type streamRouteBlock struct {
	ID         string         `yaml:"id"`
	ServerAddr string         `yaml:"server_addr,omitempty"`
	ServerPort int            `yaml:"server_port"`
	Plugins    map[string]any `yaml:"plugins,omitempty"`
	UpstreamID string         `yaml:"upstream_id"`
}

type protoBlock struct {
	ID      string `yaml:"id"`
	Desc    string `yaml:"desc,omitempty"`
	Content string `yaml:"content"`
}

type deferredAdapter struct {
	ID          string
	Protocol    string
	PublicPath  string
	ResolverURL string
	RuntimeURL  string
	Description string
}

type resolverBinding struct {
	ID          string
	Protocol    string
	PublicPath  string
	ResolverURL string
	RuntimeURL  string
}
