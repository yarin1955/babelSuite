package apisix

import (
	"net/url"
	"strings"
)

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
