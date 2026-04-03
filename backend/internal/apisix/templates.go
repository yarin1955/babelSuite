package apisix

import "fmt"

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
		fmt.Sprintf("note: replace the placeholder query with the GraphQL schema-bound operation for %s", firstNonEmpty(surface.ID, operation.ID)),
	}
}

func renderCommentedBlock(lines []string) []string {
	output := make([]string, 0, len(lines)+1)
	for _, line := range lines {
		if line == "" {
			output = append(output, "#")
			continue
		}
		output = append(output, "# "+line)
	}
	return append(output, "#")
}
