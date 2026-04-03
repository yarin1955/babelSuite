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

func RenderStandaloneConfig(suite SuiteConfig) string {
	routes, streamRoutes, upstreams, protos, deferred := buildResources(suite)
	resolvers := buildResolverBindings(suite)
	pluginTemplates := buildProtocolTemplates(suite)
	document := routeDocument{
		Deployment: deploymentBlock{
			Role: "data_plane",
			RoleDataPlane: deploymentDataPlane{
				ConfigProvider: "yaml",
			},
		},
		Plugins:      buildPluginCatalog(suite),
		Protos:       protos,
		Upstreams:    upstreams,
		Routes:       routes,
		StreamRoutes: streamRoutes,
	}

	body, err := yaml.Marshal(document)
	if err != nil {
		return "deployment:\n  role: data_plane\n  role_data_plane:\n    config_provider: yaml\nroutes: []\n#END\n"
	}

	lines := []string{
		strings.TrimRight(string(body), "\n"),
		"",
		"# Set BABELSUITE_ENGINE_ADDR to the in-agent BabelSuite engine endpoint, for example babelsuite-engine:8090.",
		"# HTTP-family routes below keep a proxy-rewrite compatibility path so suites still run while the APISIX sidecar rolls out protocol-specific plugins.",
		"# gRPC and Kafka surfaces now render as live APISIX routes, while MQTT/TCP/UDP surfaces render as live APISIX stream routes and shared upstream objects.",
		"# Query parameters continue to flow to the engine unchanged so mock dispatch still happens in BabelSuite.",
	}

	if len(streamRoutes) > 0 {
		lines = append(lines,
			"#",
			"# Stream-style transports such as MQTT/TCP/UDP require APISIX stream listeners to be enabled on the agent sidecar before the generated stream routes can accept traffic.",
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
			"# Transports below still need APISIX-side plugin-runner or custom sidecar wiring before they can call the resolver and emit the final protocol response:",
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
			"# Optional APISIX snippets below cover transports that still need sidecar-specific plugin wiring:",
		)
		lines = append(lines, pluginTemplates...)
	}

	lines = append(lines, "#END")
	return strings.Join(lines, "\n") + "\n"
}

func buildResources(suite SuiteConfig) ([]routeBlock, []streamRouteBlock, []namedUpstreamBlock, []protoBlock, []deferredAdapter) {
	routes := make([]routeBlock, 0)
	streamRoutes := make([]streamRouteBlock, 0)
	deferred := make([]deferredAdapter, 0)
	upstreamByID := make(map[string]namedUpstreamBlock)
	protoByID := make(map[string]protoBlock)
	streamPorts := map[string]int{
		"mqtt": 9100,
		"tcp":  9200,
		"udp":  9300,
	}

	for _, surface := range suite.APISurfaces {
		for _, operation := range surface.Operations {
			transport := transportKind(surface, operation)
			switch transport {
			case "rest", "soap", "graphql", "websocket", "sse", "webhook":
				routes = append(routes, httpCompatibleRoute(suite, surface, operation, transport))
			case "grpc":
				route, proto, ok := grpcRoute(suite, surface, operation)
				if !ok {
					deferred = append(deferred, newDeferredAdapter(suite, surface, operation, transport))
					continue
				}
				routes = append(routes, route)
				protoByID[proto.ID] = proto
			case "kafka":
				routes = append(routes, kafkaRoute(suite, surface, operation))
			case "mqtt", "tcp", "udp":
				streamPorts[transport]++
				streamRoute, upstream := streamRouteResource(suite, surface, operation, transport, streamPorts[transport]-1)
				streamRoutes = append(streamRoutes, streamRoute)
				upstreamByID[upstream.ID] = upstream
			default:
				deferred = append(deferred, newDeferredAdapter(suite, surface, operation, transport))
			}
		}
	}

	sort.Slice(routes, func(i, j int) bool {
		return routes[i].ID < routes[j].ID
	})
	sort.Slice(streamRoutes, func(i, j int) bool {
		return streamRoutes[i].ID < streamRoutes[j].ID
	})
	sort.Slice(deferred, func(i, j int) bool {
		return deferred[i].ID < deferred[j].ID
	})

	upstreamIDs := make([]string, 0, len(upstreamByID))
	for id := range upstreamByID {
		upstreamIDs = append(upstreamIDs, id)
	}
	sort.Strings(upstreamIDs)

	upstreams := make([]namedUpstreamBlock, 0, len(upstreamIDs))
	for _, id := range upstreamIDs {
		upstreams = append(upstreams, upstreamByID[id])
	}

	protoIDs := make([]string, 0, len(protoByID))
	for id := range protoByID {
		protoIDs = append(protoIDs, id)
	}
	sort.Strings(protoIDs)

	protos := make([]protoBlock, 0, len(protoIDs))
	for _, id := range protoIDs {
		protos = append(protos, protoByID[id])
	}

	return routes, streamRoutes, upstreams, protos, deferred
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
			if transport == "grpc" {
				seen["grpc-transcode"] = pluginSpec{Name: "grpc-transcode"}
				seen["ext-plugin-pre-req"] = pluginSpec{Name: "ext-plugin-pre-req"}
			}
			if transport == "graphql" {
				seen["degraphql"] = pluginSpec{Name: "degraphql"}
			}
			switch transport {
			case "kafka":
				seen["kafka-proxy"] = pluginSpec{Name: "kafka-proxy"}
			case "mqtt":
				seen["mqtt-proxy"] = pluginSpec{Name: "mqtt-proxy", Stream: true}
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
			transport := transportKind(surface, operation)
			output = append(output, resolverBinding{
				ID:          suite.ID + "." + operation.ID,
				Protocol:    transportDisplayName(transport),
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
			case "graphql":
				lines = append(lines, renderCommentedBlock(graphqlTemplateBlock(surface, operation))...)
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

func transportDisplayName(transport string) string {
	switch transport {
	case "rest":
		return "REST"
	case "soap":
		return "SOAP"
	case "graphql":
		return "GraphQL"
	case "websocket":
		return "WebSocket"
	case "sse":
		return "SSE"
	case "grpc":
		return "gRPC"
	case "kafka":
		return "Kafka"
	case "mqtt":
		return "MQTT"
	case "amqp":
		return "AMQP"
	case "nats":
		return "NATS"
	case "tcp":
		return "TCP"
	case "udp":
		return "UDP"
	case "webhook":
		return "Webhook"
	default:
		return "Async"
	}
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

func resolverPlugin(transport string, operation OperationConfig) map[string]any {
	value, _ := json.Marshal(map[string]string{
		"resolver_url":      resolverPath(operation.MockMetadata.ResolverURL),
		"public_path":       publicPath(operation),
		"protocol":          strings.ToUpper(strings.TrimSpace(transport)),
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

func httpCompatibleRoute(suite SuiteConfig, surface SurfaceConfig, operation OperationConfig, transport string) routeBlock {
	return routeBlock{
		ID:              suite.ID + "." + operation.ID,
		Name:            operation.ID,
		Desc:            strings.TrimSpace(operation.Summary),
		URI:             matchURI(operation),
		Methods:         []string{httpMethod(operation)},
		Hosts:           routeHosts(surface),
		EnableWebsocket: transport == "websocket",
		Plugins: map[string]any{
			"ext-plugin-pre-req": resolverPlugin(transport, operation),
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
}

func grpcRoute(suite SuiteConfig, surface SurfaceConfig, operation OperationConfig) (routeBlock, protoBlock, bool) {
	contract := strings.TrimSpace(operation.ContractContent)
	if contract == "" {
		return routeBlock{}, protoBlock{}, false
	}

	protoID := suite.ID + "." + operation.ID
	serviceName, methodName := grpcServiceMethod(operation.Name)
	return routeBlock{
			ID:      protoID + ".grpc",
			Name:    operation.ID,
			Desc:    strings.TrimSpace(operation.Summary),
			URI:     publicPath(operation),
			Methods: []string{"POST"},
			Hosts:   routeHosts(surface),
			Plugins: map[string]any{
				"grpc-transcode": map[string]any{
					"proto_id": protoID,
					"service":  serviceName,
					"method":   methodName,
				},
				"ext-plugin-pre-req": resolverPlugin("grpc", operation),
			},
			Upstream: upstreamBlock{
				Type:   "roundrobin",
				Scheme: "grpc",
				Nodes: map[string]int{
					grpcUpstreamAddrEnv: 1,
				},
			},
		}, protoBlock{
			ID:      protoID,
			Desc:    firstNonEmpty(contractSourcePath(operation.ContractPath), operation.ID),
			Content: ensureTrailingNewline(contract),
		}, true
}

func kafkaRoute(suite SuiteConfig, surface SurfaceConfig, operation OperationConfig) routeBlock {
	return routeBlock{
		ID:      suite.ID + "." + operation.ID + ".kafka",
		Name:    operation.ID,
		Desc:    strings.TrimSpace(operation.Summary),
		URI:     publicPath(operation),
		Methods: []string{httpMethod(operation)},
		Hosts:   routeHosts(surface),
		Plugins: map[string]any{
			"kafka-proxy": map[string]any{
				"sasl": map[string]string{
					"username": "${{BABELSUITE_KAFKA_USERNAME}}",
					"password": "${{BABELSUITE_KAFKA_PASSWORD}}",
				},
			},
			"ext-plugin-pre-req": resolverPlugin("kafka", operation),
		},
		Upstream: upstreamBlock{
			Type:   "roundrobin",
			Scheme: "kafka",
			Nodes: map[string]int{
				kafkaUpstreamAddrEnv: 1,
			},
		},
	}
}

func streamRouteResource(suite SuiteConfig, _ SurfaceConfig, operation OperationConfig, transport string, serverPort int) (streamRouteBlock, namedUpstreamBlock) {
	id := suite.ID + "." + operation.ID + "." + transport
	route := streamRouteBlock{
		ID:         id,
		ServerPort: serverPort,
		UpstreamID: id,
	}

	upstream := namedUpstreamBlock{
		ID:    id,
		Type:  "roundrobin",
		Nodes: map[string]int{},
	}

	switch transport {
	case "mqtt":
		route.Plugins = map[string]any{
			"mqtt-proxy": map[string]any{
				"protocol_name":  "MQTT",
				"protocol_level": 4,
			},
		}
		upstream.Nodes[mqttUpstreamAddrEnv] = 1
	case "tcp":
		upstream.Scheme = "tcp"
		upstream.Nodes[tcpUpstreamAddrEnv] = 1
	case "udp":
		upstream.Scheme = "udp"
		upstream.Nodes[udpUpstreamAddrEnv] = 1
	}

	return route, upstream
}

func newDeferredAdapter(suite SuiteConfig, surface SurfaceConfig, operation OperationConfig, transport string) deferredAdapter {
	return deferredAdapter{
		ID:          suite.ID + "." + operation.ID,
		Protocol:    transportDisplayName(transport),
		PublicPath:  publicPath(operation),
		ResolverURL: resolverPath(operation.MockMetadata.ResolverURL),
		RuntimeURL:  runtimePath(operation.MockMetadata.RuntimeURL),
		Description: strings.TrimSpace(operation.MockMetadata.DispatcherRules),
	}
}

func contractSourcePath(contractPath string) string {
	base, _, _ := strings.Cut(strings.TrimSpace(contractPath), "#")
	return strings.TrimSpace(base)
}

func ensureTrailingNewline(value string) string {
	if value == "" || strings.HasSuffix(value, "\n") {
		return value
	}
	return value + "\n"
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
