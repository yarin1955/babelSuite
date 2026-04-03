package apisix

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const engineAddrEnv = "${{BABELSUITE_ENGINE_ADDR}}"

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
	ID           string
	Method       string
	Name         string
	Summary      string
	ContractPath string
	MockURL      string
	MockMetadata OperationMetadataConfig
}

type OperationMetadataConfig struct {
	Adapter         string
	DispatcherRules string
	ResolverURL     string
	RuntimeURL      string
}

type routeDocument struct {
	Deployment deploymentBlock `yaml:"deployment"`
	Plugins    []pluginSpec    `yaml:"plugins,omitempty"`
	Routes     []routeBlock    `yaml:"routes"`
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

func RenderStandaloneConfig(suite SuiteConfig) string {
	routes, deferred := buildRoutes(suite)
	resolvers := buildResolverBindings(suite)
	pluginTemplates := buildProtocolTemplates(suite)
	document := routeDocument{
		Deployment: deploymentBlock{
			Role: "data_plane",
			RoleDataPlane: deploymentDataPlane{
				ConfigProvider: "yaml",
			},
		},
		Plugins: buildPluginCatalog(suite),
		Routes:  routes,
	}

	body, err := yaml.Marshal(document)
	if err != nil {
		return "deployment:\n  role: data_plane\n  role_data_plane:\n    config_provider: yaml\nroutes: []\n#END\n"
	}

	lines := []string{
		strings.TrimRight(string(body), "\n"),
		"",
		"# Set BABELSUITE_ENGINE_ADDR to the in-agent BabelSuite engine endpoint, for example babelsuite-engine:8090.",
		"# HTTP-family routes below keep a proxy-rewrite compatibility path so suites still run before the APISIX sidecar enables protocol-specific plugins.",
		"# Query parameters continue to flow to the engine unchanged so mock dispatch still happens in BabelSuite.",
	}

	if suiteHasStreamTransports(suite) {
		lines = append(lines,
			"#",
			"# Stream-style transports such as MQTT/TCP/UDP require APISIX stream listeners to be enabled on the agent sidecar before the templates below can be activated.",
		)
	}

	if len(resolvers) > 0 {
		lines = append(lines,
			"#",
			"# Resolver contracts below describe the internal BabelSuite endpoint APISIX should call to fetch normalized mock data:",
		)
		for _, item := range resolvers {
			lines = append(lines,
				fmt.Sprintf("# - %s (%s) public=%s resolver=%s compatibility=%s", item.ID, item.Protocol, item.PublicPath, item.ResolverURL, item.RuntimeURL),
			)
		}
	}

	if len(deferred) > 0 {
		lines = append(lines,
			"#",
			"# Non-HTTP transports below still need APISIX-native plugin or stream wiring before they can call the resolver and emit the final protocol response:",
		)
		for _, item := range deferred {
			lines = append(lines,
				fmt.Sprintf("# - %s (%s) public=%s resolver=%s compatibility=%s", item.ID, item.Protocol, item.PublicPath, item.ResolverURL, item.RuntimeURL),
				fmt.Sprintf("#   %s", item.Description),
			)
		}
	}

	if len(pluginTemplates) > 0 {
		lines = append(lines,
			"#",
			"# Protocol plugin templates below show the APISIX-native plugins to use when you enable deferred transport support in the sidecar:",
		)
		lines = append(lines, pluginTemplates...)
	}

	lines = append(lines, "#END")
	return strings.Join(lines, "\n") + "\n"
}

func buildRoutes(suite SuiteConfig) ([]routeBlock, []deferredAdapter) {
	routes := make([]routeBlock, 0)
	deferred := make([]deferredAdapter, 0)

	for _, surface := range suite.APISurfaces {
		for _, operation := range surface.Operations {
			transport := transportKind(surface, operation)
			if !isHTTPCompatibleTransport(transport) {
				deferred = append(deferred, deferredAdapter{
					ID:          suite.ID + "." + operation.ID,
					Protocol:    firstNonEmpty(strings.TrimSpace(surface.Protocol), strings.ToUpper(strings.TrimSpace(operation.MockMetadata.Adapter))),
					PublicPath:  publicPath(operation),
					ResolverURL: resolverPath(operation.MockMetadata.ResolverURL),
					RuntimeURL:  runtimePath(operation.MockMetadata.RuntimeURL),
					Description: strings.TrimSpace(operation.MockMetadata.DispatcherRules),
				})
				continue
			}

			route := routeBlock{
				ID:              suite.ID + "." + operation.ID,
				Name:            operation.ID,
				Desc:            strings.TrimSpace(operation.Summary),
				URI:             matchURI(operation),
				Methods:         []string{httpMethod(operation)},
				Hosts:           routeHosts(surface),
				EnableWebsocket: transport == "websocket",
				Plugins: map[string]any{
					"ext-plugin-pre-req": resolverPlugin(surface, operation),
					"proxy-rewrite":      proxyRewrite(suite.ID, surface.ID, operation),
				},
				Upstream: upstreamBlock{
					Type:   "roundrobin",
					Scheme: "http",
					Nodes: map[string]int{
						engineAddrEnv: 1,
					},
				},
			}
			routes = append(routes, route)
		}
	}

	sort.Slice(routes, func(i, j int) bool {
		return routes[i].ID < routes[j].ID
	})
	sort.Slice(deferred, func(i, j int) bool {
		return deferred[i].ID < deferred[j].ID
	})

	return routes, deferred
}

func buildPluginCatalog(suite SuiteConfig) []pluginSpec {
	seen := map[string]pluginSpec{}
	for _, surface := range suite.APISurfaces {
		for _, operation := range surface.Operations {
			transport := transportKind(surface, operation)
			if isHTTPCompatibleTransport(transport) {
				seen["proxy-rewrite"] = pluginSpec{Name: "proxy-rewrite"}
				seen["ext-plugin-pre-req"] = pluginSpec{Name: "ext-plugin-pre-req"}
			}
			if transport == "graphql" {
				seen["degraphql"] = pluginSpec{Name: "degraphql"}
				seen["ext-plugin-pre-req"] = pluginSpec{Name: "ext-plugin-pre-req"}
			}
			if transport == "grpc" {
				seen["grpc-transcode"] = pluginSpec{Name: "grpc-transcode"}
				seen["ext-plugin-pre-req"] = pluginSpec{Name: "ext-plugin-pre-req"}
			}
			if transport == "async" || transport == "kafka" || transport == "mqtt" || transport == "amqp" || transport == "nats" || transport == "tcp" || transport == "udp" {
				seen["ext-plugin-pre-req"] = pluginSpec{Name: "ext-plugin-pre-req"}
				switch transport {
				case "kafka":
					seen["kafka-proxy"] = pluginSpec{Name: "kafka-proxy"}
				case "mqtt":
					seen["mqtt-proxy"] = pluginSpec{Name: "mqtt-proxy", Stream: true}
				}
			}
		}
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)

	output := make([]pluginSpec, 0, len(names))
	for _, name := range names {
		output = append(output, seen[name])
	}
	return output
}

func buildResolverBindings(suite SuiteConfig) []resolverBinding {
	output := make([]resolverBinding, 0)
	for _, surface := range suite.APISurfaces {
		for _, operation := range surface.Operations {
			output = append(output, resolverBinding{
				ID:          suite.ID + "." + operation.ID,
				Protocol:    firstNonEmpty(strings.TrimSpace(surface.Protocol), strings.ToUpper(strings.TrimSpace(operation.MockMetadata.Adapter))),
				PublicPath:  publicPath(operation),
				ResolverURL: resolverPath(operation.MockMetadata.ResolverURL),
				RuntimeURL:  runtimePath(operation.MockMetadata.RuntimeURL),
			})
		}
	}

	sort.Slice(output, func(i, j int) bool {
		return output[i].ID < output[j].ID
	})
	return output
}

func buildProtocolTemplates(suite SuiteConfig) []string {
	lines := make([]string, 0)
	for _, surface := range suite.APISurfaces {
		for _, operation := range surface.Operations {
			switch transportKind(surface, operation) {
			case "grpc":
				lines = append(lines, renderCommentedBlock(grpcTemplateBlock(suite, surface, operation))...)
			case "kafka":
				lines = append(lines, renderCommentedBlock(kafkaTemplateBlock(suite, surface, operation))...)
			case "mqtt":
				lines = append(lines, renderCommentedBlock(mqttTemplateBlock(suite, surface, operation))...)
			case "graphql":
				lines = append(lines, renderCommentedBlock(graphqlTemplateBlock(surface, operation))...)
			case "websocket":
				lines = append(lines, renderCommentedBlock(websocketTemplateBlock(suite, surface, operation))...)
			case "sse":
				lines = append(lines, renderCommentedBlock(sseTemplateBlock(suite, surface, operation))...)
			case "amqp":
				lines = append(lines, renderCommentedBlock(amqpTemplateBlock(suite, surface, operation))...)
			case "nats":
				lines = append(lines, renderCommentedBlock(natsTemplateBlock(suite, surface, operation))...)
			case "tcp":
				lines = append(lines, renderCommentedBlock(tcpTemplateBlock(suite, surface, operation))...)
			case "udp":
				lines = append(lines, renderCommentedBlock(udpTemplateBlock(suite, surface, operation))...)
			case "async":
				lines = append(lines, renderCommentedBlock(asyncTemplateBlock(suite, surface, operation))...)
			}
		}
	}
	return lines
}

func transportKind(surface SurfaceConfig, operation OperationConfig) string {
	protocol := normalizeTransport(strings.TrimSpace(surface.Protocol))
	switch protocol {
	case "async":
		if scheme := asyncTransportScheme(surface, operation); scheme != "" {
			return scheme
		}
		return "async"
	case "rest", "soap", "graphql", "websocket", "sse", "grpc", "kafka", "mqtt", "amqp", "nats", "tcp", "udp", "webhook":
		return protocol
	}

	adapter := normalizeTransport(strings.TrimSpace(operation.MockMetadata.Adapter))
	switch adapter {
	case "grpc":
		return "grpc"
	case "async":
		if scheme := asyncTransportScheme(surface, operation); scheme != "" {
			return scheme
		}
		return "async"
	case "rest":
		return "rest"
	}

	switch normalizeTransport(hostScheme(surface.MockHost, operation.MockURL)) {
	case "kafka", "mqtt", "amqp", "nats", "grpc", "tcp", "udp":
		return normalizeTransport(hostScheme(surface.MockHost, operation.MockURL))
	case "websocket":
		return "websocket"
	case "sse":
		return "sse"
	case "graphql":
		return "graphql"
	case "webhook":
		return "webhook"
	default:
		return "rest"
	}
}

func normalizeTransport(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "http", "https":
		return "rest"
	case "grpcs":
		return "grpc"
	case "mqtts":
		return "mqtt"
	case "amqps":
		return "amqp"
	case "ws", "wss", "websocket":
		return "websocket"
	case "graphql-ws":
		return "graphql"
	case "webhooks":
		return "webhook"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func asyncTransportScheme(surface SurfaceConfig, operation OperationConfig) string {
	switch scheme := normalizeTransport(hostScheme(surface.MockHost, operation.MockURL)); scheme {
	case "kafka", "mqtt", "amqp", "nats", "tcp", "udp":
		return scheme
	default:
		return ""
	}
}

func isHTTPCompatibleTransport(transport string) bool {
	switch transport {
	case "rest", "soap", "graphql", "websocket", "sse", "webhook":
		return true
	default:
		return false
	}
}

func suiteHasStreamTransports(suite SuiteConfig) bool {
	for _, surface := range suite.APISurfaces {
		for _, operation := range surface.Operations {
			switch transportKind(surface, operation) {
			case "mqtt", "tcp", "udp":
				return true
			}
		}
	}
	return false
}

func publicPath(operation OperationConfig) string {
	if strings.HasPrefix(strings.TrimSpace(operation.Name), "/") {
		return strings.TrimSpace(operation.Name)
	}
	if raw := strings.TrimSpace(operation.MockURL); raw != "" {
		if parsed, err := url.Parse(raw); err == nil && strings.TrimSpace(parsed.Path) != "" {
			return parsed.Path
		}
	}
	if strings.TrimSpace(operation.Name) != "" {
		return "/" + strings.Trim(strings.TrimSpace(operation.Name), "/")
	}
	return "/" + strings.TrimSpace(operation.ID)
}

func matchURI(operation OperationConfig) string {
	path := publicPath(operation)
	if !hasPathParams(path) {
		return path
	}

	replaced := pathParameterPattern.ReplaceAllString(path, "*")
	if strings.TrimSpace(replaced) == "" {
		return "/*"
	}
	return replaced
}

func proxyRewrite(suiteID, surfaceID string, operation OperationConfig) map[string]any {
	headers := map[string]string{
		"X-Babelsuite-Dispatcher": "apisix",
		"X-Babelsuite-Operation":  operation.ID,
	}

	target := runtimeTargetPath(suiteID, surfaceID, operation)
	if !hasPathParams(publicPath(operation)) {
		return map[string]any{
			"uri": target,
			"headers": map[string]any{
				"set": headers,
			},
		}
	}

	pattern, replacement := rewritePattern(publicPath(operation), target)
	return map[string]any{
		"regex_uri": []string{pattern, replacement},
		"headers": map[string]any{
			"set": headers,
		},
	}
}

func resolverPlugin(surface SurfaceConfig, operation OperationConfig) map[string]any {
	value, _ := json.Marshal(map[string]string{
		"resolver_url":      resolverPath(operation.MockMetadata.ResolverURL),
		"public_path":       publicPath(operation),
		"protocol":          strings.ToUpper(strings.TrimSpace(surface.Protocol)),
		"operation_id":      operation.ID,
		"compatibility_url": runtimePath(operation.MockMetadata.RuntimeURL),
	})
	return map[string]any{
		"allow_degradation": true,
		"conf": []map[string]any{
			{
				"name":  "babelsuite-resolver",
				"value": string(value),
			},
		},
	}
}

func httpMethod(operation OperationConfig) string {
	method := strings.ToUpper(strings.TrimSpace(operation.Method))
	if method == "" || method == "RPC" || method == "EVENT" {
		return "POST"
	}
	return method
}

func routeHosts(surface SurfaceConfig) []string {
	host := strings.TrimSpace(hostOnly(surface.MockHost))
	if host == "" {
		return nil
	}
	return []string{host}
}

func runtimePath(runtimeURL string) string {
	raw := strings.TrimSpace(runtimeURL)
	if raw == "" {
		return "/"
	}
	if parsed, err := url.Parse(raw); err == nil && strings.TrimSpace(parsed.Path) != "" {
		return parsed.Path
	}
	if index := strings.Index(raw, "?"); index >= 0 {
		return raw[:index]
	}
	return raw
}

func resolverPath(resolverURL string) string {
	return runtimePath(resolverURL)
}

func hostOnly(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if parsed, err := url.Parse(trimmed); err == nil {
		if host := strings.TrimSpace(parsed.Host); host != "" {
			return host
		}
	}
	return strings.Trim(trimmed, "/")
}

func hasPathParams(path string) bool {
	return pathParameterPattern.MatchString(path)
}

func rewritePattern(publicPath, targetPath string) (string, string) {
	matches := pathParameterPattern.FindAllString(publicPath, -1)
	pattern := "^" + pathParameterPattern.ReplaceAllString(publicPath, "([^/]+)") + "$"
	replacement := targetPath
	for index := range matches {
		replacement = strings.Replace(replacement, matches[index], fmt.Sprintf("$%d", index+1), 1)
	}

	return pattern, replacement
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func runtimeTargetPath(suiteID, surfaceID string, operation OperationConfig) string {
	path := strings.TrimSpace(operation.Name)
	if path == "" {
		path = publicPath(operation)
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + strings.Trim(path, "/")
	}
	return "/mocks/rest/" + strings.Trim(suiteID, "/") + "/" + strings.Trim(surfaceID, "/") + path
}

func grpcTemplateBlock(suite SuiteConfig, surface SurfaceConfig, operation OperationConfig) []string {
	serviceName, methodName := grpcServiceMethod(operation.Name)
	return []string{
		fmt.Sprintf("template: %s.%s.grpc-transcode", suite.ID, operation.ID),
		"route:",
		fmt.Sprintf("  id: %s.%s.grpc", suite.ID, operation.ID),
		fmt.Sprintf("  uri: %s", publicPath(operation)),
		"  methods:",
		"    - POST",
		"  plugins:",
		"    grpc-transcode:",
		fmt.Sprintf("      proto_id: %s.%s", suite.ID, operation.ID),
		fmt.Sprintf("      service: %s", serviceName),
		fmt.Sprintf("      method: %s", methodName),
		"    ext-plugin-pre-req:",
		"      allow_degradation: true",
		"      conf:",
		"        - name: babelsuite-resolver",
		fmt.Sprintf("          value: '{\"resolver_url\":\"%s\",\"transport\":\"grpc\",\"operation_id\":\"%s\"}'", resolverPath(operation.MockMetadata.ResolverURL), operation.ID),
		"  upstream:",
		"    scheme: grpc",
		"    type: roundrobin",
		"    nodes:",
		"      \"${{APISIX_GRPC_UPSTREAM_ADDR}}\": 1",
		fmt.Sprintf("note: register %s as the grpc-transcode proto payload before enabling this route", firstNonEmpty(strings.TrimSpace(operation.ContractPath), "api/proto/*.proto")),
	}
}

func kafkaTemplateBlock(suite SuiteConfig, _ SurfaceConfig, operation OperationConfig) []string {
	return []string{
		fmt.Sprintf("template: %s.%s.kafka-proxy", suite.ID, operation.ID),
		"route:",
		fmt.Sprintf("  id: %s.%s.kafka", suite.ID, operation.ID),
		fmt.Sprintf("  uri: %s", publicPath(operation)),
		"  plugins:",
		"    kafka-proxy:",
		"      sasl:",
		"        username: \"${{BABELSUITE_KAFKA_USERNAME}}\"",
		"        password: \"${{BABELSUITE_KAFKA_PASSWORD}}\"",
		"    ext-plugin-pre-req:",
		"      allow_degradation: true",
		"      conf:",
		"        - name: babelsuite-resolver",
		fmt.Sprintf("          value: '{\"resolver_url\":\"%s\",\"transport\":\"kafka\",\"operation_id\":\"%s\"}'", resolverPath(operation.MockMetadata.ResolverURL), operation.ID),
		"  upstream:",
		"    scheme: kafka",
		"    type: roundrobin",
		"    nodes:",
		"      \"${{APISIX_KAFKA_UPSTREAM_ADDR}}\": 1",
		"note: let APISIX own the Kafka-facing route and resolve the mock payload from BabelSuite before it emits the broker message",
	}
}

func mqttTemplateBlock(suite SuiteConfig, _ SurfaceConfig, operation OperationConfig) []string {
	return []string{
		fmt.Sprintf("template: %s.%s.mqtt-proxy", suite.ID, operation.ID),
		"stream_route:",
		fmt.Sprintf("  id: %s.%s.mqtt", suite.ID, operation.ID),
		"  server_port: 9100",
		"  plugins:",
		"    mqtt-proxy:",
		"      protocol_name: MQTT",
		"      protocol_level: 4",
		"  upstream:",
		"    type: chash",
		"    key: mqtt_client_id",
		"    nodes:",
		"      - host: \"${{APISIX_MQTT_UPSTREAM_ADDR}}\"",
		"        port: 1883",
		"        weight: 1",
		fmt.Sprintf("note: let APISIX stream mode resolve MQTT payload data from %s before it emits the frame", resolverPath(operation.MockMetadata.ResolverURL)),
	}
}

func graphqlTemplateBlock(surface SurfaceConfig, operation OperationConfig) []string {
	return []string{
		fmt.Sprintf("template: %s.degraphql", operation.ID),
		"route:",
		fmt.Sprintf("  uri: %s", publicPath(operation)),
		"  plugins:",
		"    degraphql:",
		"      query: \"query { __typename }\"",
		"    ext-plugin-pre-req:",
		"      allow_degradation: true",
		"      conf:",
		"        - name: babelsuite-resolver",
		fmt.Sprintf("          value: '{\"resolver_url\":\"%s\",\"transport\":\"graphql\",\"operation_id\":\"%s\"}'", resolverPath(operation.MockMetadata.ResolverURL), operation.ID),
		fmt.Sprintf("note: replace the placeholder query with the GraphQL schema-bound operation for %s", firstNonEmpty(strings.TrimSpace(surface.ID), operation.ID)),
	}
}

func websocketTemplateBlock(suite SuiteConfig, _ SurfaceConfig, operation OperationConfig) []string {
	return []string{
		fmt.Sprintf("template: %s.%s.websocket", suite.ID, operation.ID),
		"route:",
		fmt.Sprintf("  id: %s.%s.websocket", suite.ID, operation.ID),
		fmt.Sprintf("  uri: %s", publicPath(operation)),
		"  enable_websocket: true",
		"  plugins:",
		"    ext-plugin-pre-req:",
		"      allow_degradation: true",
		"      conf:",
		"        - name: babelsuite-resolver",
		fmt.Sprintf("          value: '{\"resolver_url\":\"%s\",\"transport\":\"websocket\",\"operation_id\":\"%s\"}'", resolverPath(operation.MockMetadata.ResolverURL), operation.ID),
		"  upstream:",
		"    scheme: http",
		"    type: roundrobin",
		"    nodes:",
		"      \"${{APISIX_WEBSOCKET_UPSTREAM_ADDR}}\": 1",
		"note: keep APISIX as the websocket terminator and let the sidecar upstream use BabelSuite resolver output for session state and canned frames",
	}
}

func sseTemplateBlock(suite SuiteConfig, _ SurfaceConfig, operation OperationConfig) []string {
	return []string{
		fmt.Sprintf("template: %s.%s.sse", suite.ID, operation.ID),
		"route:",
		fmt.Sprintf("  id: %s.%s.sse", suite.ID, operation.ID),
		fmt.Sprintf("  uri: %s", publicPath(operation)),
		"  methods:",
		"    - GET",
		"  plugins:",
		"    ext-plugin-pre-req:",
		"      allow_degradation: true",
		"      conf:",
		"        - name: babelsuite-resolver",
		fmt.Sprintf("          value: '{\"resolver_url\":\"%s\",\"transport\":\"sse\",\"operation_id\":\"%s\"}'", resolverPath(operation.MockMetadata.ResolverURL), operation.ID),
		"  upstream:",
		"    scheme: http",
		"    type: roundrobin",
		"    nodes:",
		"      \"${{APISIX_SSE_UPSTREAM_ADDR}}\": 1",
		"note: keep the event stream open in APISIX while the sidecar upstream pulls BabelSuite-generated events from the resolver",
	}
}

func asyncTemplateBlock(suite SuiteConfig, _ SurfaceConfig, operation OperationConfig) []string {
	return []string{
		fmt.Sprintf("template: %s.%s.async-adapter", suite.ID, operation.ID),
		"route:",
		fmt.Sprintf("  id: %s.%s.async", suite.ID, operation.ID),
		fmt.Sprintf("  uri: %s", publicPath(operation)),
		"  plugins:",
		"    ext-plugin-pre-req:",
		"      allow_degradation: true",
		"      conf:",
		"        - name: babelsuite-resolver",
		fmt.Sprintf("          value: '{\"resolver_url\":\"%s\",\"transport\":\"async\",\"operation_id\":\"%s\"}'", resolverPath(operation.MockMetadata.ResolverURL), operation.ID),
		"note: use APISIX plugin-runner wiring here when the async transport is not tied to a built-in broker plugin",
	}
}

func amqpTemplateBlock(suite SuiteConfig, _ SurfaceConfig, operation OperationConfig) []string {
	return []string{
		fmt.Sprintf("template: %s.%s.amqp-adapter", suite.ID, operation.ID),
		"route:",
		fmt.Sprintf("  id: %s.%s.amqp", suite.ID, operation.ID),
		fmt.Sprintf("  uri: %s", publicPath(operation)),
		"  plugins:",
		"    ext-plugin-pre-req:",
		"      allow_degradation: true",
		"      conf:",
		"        - name: babelsuite-resolver",
		fmt.Sprintf("          value: '{\"resolver_url\":\"%s\",\"transport\":\"amqp\",\"operation_id\":\"%s\"}'", resolverPath(operation.MockMetadata.ResolverURL), operation.ID),
		"note: wire this route through APISIX plugin-runner support because APISIX does not ship a dedicated AMQP mock responder plugin",
	}
}

func natsTemplateBlock(suite SuiteConfig, _ SurfaceConfig, operation OperationConfig) []string {
	return []string{
		fmt.Sprintf("template: %s.%s.nats-adapter", suite.ID, operation.ID),
		"route:",
		fmt.Sprintf("  id: %s.%s.nats", suite.ID, operation.ID),
		fmt.Sprintf("  uri: %s", publicPath(operation)),
		"  plugins:",
		"    ext-plugin-pre-req:",
		"      allow_degradation: true",
		"      conf:",
		"        - name: babelsuite-resolver",
		fmt.Sprintf("          value: '{\"resolver_url\":\"%s\",\"transport\":\"nats\",\"operation_id\":\"%s\"}'", resolverPath(operation.MockMetadata.ResolverURL), operation.ID),
		"note: wire this route through APISIX plugin-runner support because APISIX does not ship a dedicated NATS mock responder plugin",
	}
}

func tcpTemplateBlock(suite SuiteConfig, _ SurfaceConfig, operation OperationConfig) []string {
	return []string{
		fmt.Sprintf("template: %s.%s.tcp-stream", suite.ID, operation.ID),
		"stream_route:",
		fmt.Sprintf("  id: %s.%s.tcp", suite.ID, operation.ID),
		"  server_port: 9200",
		"  plugins:",
		"    ext-plugin-pre-req:",
		"      allow_degradation: true",
		"      conf:",
		"        - name: babelsuite-resolver",
		fmt.Sprintf("          value: '{\"resolver_url\":\"%s\",\"transport\":\"tcp\",\"operation_id\":\"%s\"}'", resolverPath(operation.MockMetadata.ResolverURL), operation.ID),
		"  upstream:",
		"    type: roundrobin",
		"    scheme: tcp",
		"    nodes:",
		"      \"${{APISIX_TCP_UPSTREAM_ADDR}}\": 1",
		"note: enable APISIX stream mode before activating TCP mock routes",
	}
}

func udpTemplateBlock(suite SuiteConfig, _ SurfaceConfig, operation OperationConfig) []string {
	return []string{
		fmt.Sprintf("template: %s.%s.udp-stream", suite.ID, operation.ID),
		"stream_route:",
		fmt.Sprintf("  id: %s.%s.udp", suite.ID, operation.ID),
		"  server_port: 9300",
		"  plugins:",
		"    ext-plugin-pre-req:",
		"      allow_degradation: true",
		"      conf:",
		"        - name: babelsuite-resolver",
		fmt.Sprintf("          value: '{\"resolver_url\":\"%s\",\"transport\":\"udp\",\"operation_id\":\"%s\"}'", resolverPath(operation.MockMetadata.ResolverURL), operation.ID),
		"  upstream:",
		"    type: roundrobin",
		"    scheme: udp",
		"    nodes:",
		"      \"${{APISIX_UDP_UPSTREAM_ADDR}}\": 1",
		"note: enable APISIX stream mode before activating UDP mock routes",
	}
}

func renderCommentedBlock(lines []string) []string {
	output := make([]string, 0, len(lines)+1)
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			output = append(output, "#")
			continue
		}
		output = append(output, "# "+line)
	}
	return append(output, "#")
}

func grpcServiceMethod(name string) (string, string) {
	trimmed := strings.Trim(strings.TrimSpace(name), "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) >= 2 {
		return parts[0], parts[1]
	}
	if len(parts) == 1 {
		return parts[0], "Invoke"
	}
	return "grpc.Service", "Invoke"
}

func hostScheme(values ...string) string {
	for _, raw := range values {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		if parsed, err := url.Parse(trimmed); err == nil && strings.TrimSpace(parsed.Scheme) != "" {
			return parsed.Scheme
		}
	}
	return ""
}
