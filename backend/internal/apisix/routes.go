package apisix

import "strings"

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
