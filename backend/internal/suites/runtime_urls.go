package suites

import (
	"sort"
	"strings"
)

func runtimeURLForOperation(suiteID string, surface APISurface, operation APIOperation) string {
	base := "/mocks/" + defaultMockAdapter(surface.Protocol) + "/" + suiteID + "/" + surface.ID
	switch defaultMockAdapter(surface.Protocol) {
	case "grpc", "async":
		return base + "/" + operation.ID
	default:
		path := operation.Name
		if !strings.HasPrefix(path, "/") {
			path = "/" + sanitizeIdentifier(path)
		}
		return base + fillPathParameters(path, operation)
	}
}

func resolverURLForOperation(suiteID string, surface APISurface, operation APIOperation) string {
	return "/internal/mock-data/" + strings.Trim(suiteID, "/") + "/" + strings.Trim(surface.ID, "/") + "/" + strings.Trim(operation.ID, "/")
}

func fillPathParameters(path string, operation APIOperation) string {
	if !strings.Contains(path, "{") {
		return path + runtimeQuerySuffix(operation)
	}
	if len(operation.Exchanges) > 0 {
		for _, cond := range operation.Exchanges[0].When {
			if cond.From == "path" {
				path = strings.ReplaceAll(path, "{"+cond.Param+"}", cond.Value)
			}
		}
	}
	return path + runtimeQuerySuffix(operation)
}

func runtimeQuerySuffix(operation APIOperation) string {
	if len(operation.Exchanges) == 0 {
		return ""
	}
	query := make([]string, 0)
	for _, cond := range operation.Exchanges[0].When {
		if cond.From == "query" {
			query = append(query, cond.Param+"="+cond.Value)
		}
	}
	sort.Strings(query)
	if len(query) == 0 {
		return ""
	}
	return "?" + strings.Join(query, "&")
}
