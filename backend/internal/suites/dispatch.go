package suites

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

const defaultDispatcherName = "apisix"

func defaultMockDispatcher(_ APISurface, _ APIOperation) string {
	return defaultDispatcherName
}

func defaultDispatcherRules(suiteID string, surface APISurface, operation APIOperation, metadata MockOperationMetadata) string {
	publicPath := publicPathForOperation(operation)
	resolverPath := dispatcherResolverPath(suiteID, surface, operation, metadata)

	switch strings.ToLower(strings.TrimSpace(metadata.Adapter)) {
	case "grpc":
		return fmt.Sprintf("The APISIX sidecar should terminate or normalize gRPC traffic for %s and call %s to fetch normalized status, headers, and body so APISIX remains the visible responder while BabelSuite only resolves the matched exchange.", publicPath, resolverPath)
	case "async":
		return fmt.Sprintf("The APISIX sidecar should use a plugin-backed event adapter for %s and call %s to fetch normalized payload data so APISIX owns the broker-facing response flow while BabelSuite only generates the mock payload.", publicPath, resolverPath)
	default:
		method := strings.ToUpper(strings.TrimSpace(operation.Method))
		if method == "" || method == "RPC" || method == "EVENT" {
			method = "POST"
		}
		if strings.EqualFold(strings.TrimSpace(surface.Protocol), "SOAP") {
			return fmt.Sprintf("The APISIX sidecar should terminate SOAP envelopes on %s %s, preserve SOAPAction routing, and call %s to fetch the matched XML response while APISIX remains the public SOAP responder.", method, publicPath, resolverPath)
		}
		return fmt.Sprintf("The APISIX sidecar matches %s %s and calls %s to fetch normalized status, headers, and body while APISIX remains the public responder.", method, publicPath, resolverPath)
	}
}

func ensureGatewayFolder(suite Definition) Definition {
	if len(suite.APISurfaces) == 0 {
		return suite
	}

	const (
		folderName  = "gateway"
		fileName    = "apisix.yaml"
		description = "Runtime-managed APISIX sidecar routes that front suite APIs and forward traffic into the BabelSuite mock engine."
		defaultRole = "Extension"
	)

	for index := range suite.Folders {
		if suite.Folders[index].Name != folderName {
			continue
		}
		for _, existing := range suite.Folders[index].Files {
			if strings.TrimSpace(existing) == fileName {
				return suite
			}
		}
		suite.Folders[index].Files = append(suite.Folders[index].Files, fileName)
		sort.Strings(suite.Folders[index].Files)
		if strings.TrimSpace(suite.Folders[index].Role) == "" {
			suite.Folders[index].Role = defaultRole
		}
		if strings.TrimSpace(suite.Folders[index].Description) == "" {
			suite.Folders[index].Description = description
		}
		return suite
	}

	suite.Folders = append(suite.Folders, FolderEntry{
		Name:        folderName,
		Role:        defaultRole,
		Description: description,
		Files:       []string{fileName},
	})
	return suite
}

func publicPathForOperation(operation APIOperation) string {
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

	return "/" + sanitizeIdentifier(operation.ID)
}

func runtimePathOnly(runtimeURL string) string {
	if raw := strings.TrimSpace(runtimeURL); raw != "" {
		if parsed, err := url.Parse(raw); err == nil && strings.TrimSpace(parsed.Path) != "" {
			return parsed.Path
		}
		if index := strings.Index(raw, "?"); index >= 0 {
			return raw[:index]
		}
		return raw
	}
	return "/"
}

func dispatcherResolverPath(suiteID string, surface APISurface, operation APIOperation, metadata MockOperationMetadata) string {
	if raw := strings.TrimSpace(metadata.ResolverURL); raw != "" {
		return runtimePathOnly(raw)
	}
	return "/internal/mock-data/" + strings.Trim(suiteID, "/") + "/" + strings.Trim(surface.ID, "/") + "/" + strings.Trim(operation.ID, "/")
}
