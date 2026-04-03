package apisix

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

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
			switch transport := transportKind(surface, operation); transport {
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

	sort.Slice(routes, func(i, j int) bool { return routes[i].ID < routes[j].ID })
	sort.Slice(streamRoutes, func(i, j int) bool { return streamRoutes[i].ID < streamRoutes[j].ID })
	sort.Slice(deferred, func(i, j int) bool { return deferred[i].ID < deferred[j].ID })

	upstreams := sortedNamedUpstreams(upstreamByID)
	protos := sortedProtos(protoByID)

	return routes, streamRoutes, upstreams, protos, deferred
}

func buildPluginCatalog(suite SuiteConfig) []pluginSpec {
	seen := map[string]pluginSpec{}
	for _, surface := range suite.APISurfaces {
		for _, operation := range surface.Operations {
			transport := transportKind(surface, operation)
			switch {
			case isHTTPCompatibleTransport(transport):
				seen["proxy-rewrite"] = pluginSpec{Name: "proxy-rewrite"}
				seen["ext-plugin-pre-req"] = pluginSpec{Name: "ext-plugin-pre-req"}
			case transport == "grpc":
				seen["grpc-transcode"] = pluginSpec{Name: "grpc-transcode"}
				seen["ext-plugin-pre-req"] = pluginSpec{Name: "ext-plugin-pre-req"}
			case transport == "kafka":
				seen["kafka-proxy"] = pluginSpec{Name: "kafka-proxy"}
				seen["ext-plugin-pre-req"] = pluginSpec{Name: "ext-plugin-pre-req"}
			case transport == "mqtt":
				seen["mqtt-proxy"] = pluginSpec{Name: "mqtt-proxy", Stream: true}
			}
			if transport == "graphql" {
				seen["degraphql"] = pluginSpec{Name: "degraphql"}
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

	sort.Slice(output, func(i, j int) bool { return output[i].ID < output[j].ID })
	return output
}

func buildProtocolTemplates(suite SuiteConfig) []string {
	lines := make([]string, 0)
	for _, surface := range suite.APISurfaces {
		for _, operation := range surface.Operations {
			if transportKind(surface, operation) == "graphql" {
				lines = append(lines, renderCommentedBlock(graphqlTemplateBlock(surface, operation))...)
			}
		}
	}
	return lines
}

func sortedNamedUpstreams(items map[string]namedUpstreamBlock) []namedUpstreamBlock {
	ids := make([]string, 0, len(items))
	for id := range items {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	output := make([]namedUpstreamBlock, 0, len(ids))
	for _, id := range ids {
		output = append(output, items[id])
	}
	return output
}

func sortedProtos(items map[string]protoBlock) []protoBlock {
	ids := make([]string, 0, len(items))
	for id := range items {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	output := make([]protoBlock, 0, len(ids))
	for _, id := range ids {
		output = append(output, items[id])
	}
	return output
}
