package examplegen

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/babelsuite/babelsuite/internal/suites"
)

var cueTemplateTokenPattern = regexp.MustCompile(`(?s)\{\{\s*(.*?)\s*\}\}`)

func inferHeaderSchema(headers []suites.Header) map[string]any {
	if len(headers) == 0 {
		return map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	}

	properties := make(map[string]any, len(headers))
	required := make([]string, 0, len(headers))
	for _, header := range headers {
		name := strings.TrimSpace(header.Name)
		if name == "" {
			continue
		}
		required = append(required, name)
		properties[name] = inferJSONSchemaValue(header.Value)
	}

	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		sort.Strings(required)
		schema["required"] = required
	}
	return schema
}

func inferBodySchema(body string) any {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return map[string]any{"type": "null"}
	}

	var parsed any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
		return inferJSONSchemaValue(parsed)
	}

	if strings.HasPrefix(trimmed, "<") {
		schema := map[string]any{
			"type":   "string",
			"format": "xml",
		}
		if inferred, ok := inferStringRuleSchema("string", trimmed); ok {
			for key, value := range inferred {
				schema[key] = value
			}
		} else if strings.Contains(trimmed, "{{") {
			schema["x-babel-template"] = trimmed
		} else {
			schema["example"] = trimmed
		}
		return schema
	}

	schema := map[string]any{
		"type": "string",
	}
	if inferred, ok := inferStringRuleSchema("string", trimmed); ok {
		for key, value := range inferred {
			schema[key] = value
		}
	} else if strings.Contains(trimmed, "{{") {
		schema["x-babel-template"] = trimmed
	} else {
		schema["example"] = trimmed
	}
	return schema
}

func inferJSONSchemaValue(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		properties := make(map[string]any, len(typed))
		for key, nested := range typed {
			properties[key] = inferJSONSchemaValue(nested)
		}
		return map[string]any{
			"type":       "object",
			"properties": properties,
		}
	case []any:
		schema := map[string]any{"type": "array"}
		if len(typed) > 0 {
			schema["items"] = inferJSONSchemaValue(typed[0])
		}
		return schema
	case string:
		schema := map[string]any{"type": "string"}
		if inferred, ok := inferStringRuleSchema("string", typed); ok {
			for key, value := range inferred {
				schema[key] = value
			}
		} else if strings.Contains(typed, "{{") {
			schema["x-babel-template"] = typed
		} else {
			schema["example"] = typed
		}
		return schema
	case bool:
		return map[string]any{"type": "boolean", "example": typed}
	case float64:
		if typed == float64(int64(typed)) {
			return map[string]any{"type": "integer", "example": int64(typed)}
		}
		return map[string]any{"type": "number", "example": typed}
	case nil:
		return map[string]any{"type": "null"}
	default:
		return map[string]any{"type": "string", "example": fmt.Sprint(typed)}
	}
}

func inferStringRuleSchema(schemaType, value string) (map[string]any, bool) {
	if !strings.Contains(value, "{{") {
		return nil, false
	}

	matches := cueTemplateTokenPattern.FindAllStringSubmatchIndex(value, -1)
	if len(matches) == 1 {
		match := matches[0]
		prefix := value[:match[0]]
		suffix := value[match[1]:]
		expression := strings.TrimSpace(value[match[2]:match[3]])

		if generate, ok := inferGenerateRule(expression, prefix, suffix); ok {
			return map[string]any{
				"x-babel-generate": generate,
			}, true
		}

		if resolve, ok := inferResolveRule(expression, prefix, suffix); ok {
			return map[string]any{
				"x-babel-resolve": resolve,
			}, true
		}
	}

	if compose, ok := inferComposedStringRule(value, matches); ok {
		return map[string]any{
			"x-babel-compose": compose,
		}, true
	}

	return nil, false
}

func inferComposedStringRule(value string, matches [][]int) ([]any, bool) {
	if len(matches) == 0 {
		return nil, false
	}

	parts := make([]any, 0, len(matches)*2+1)
	lastEnd := 0
	dynamicCount := 0

	for _, match := range matches {
		if match[0] > lastEnd {
			parts = append(parts, value[lastEnd:match[0]])
		}

		expression := strings.TrimSpace(value[match[2]:match[3]])
		if generate, ok := inferGenerateRule(expression, "", ""); ok {
			parts = append(parts, map[string]any{"generate": generate})
			dynamicCount++
		} else if resolve, ok := inferResolveRule(expression, "", ""); ok {
			parts = append(parts, map[string]any{"resolve": resolve})
			dynamicCount++
		} else {
			return nil, false
		}

		lastEnd = match[1]
	}

	if lastEnd < len(value) {
		parts = append(parts, value[lastEnd:])
	}

	if dynamicCount == 0 {
		return nil, false
	}
	return parts, true
}

func inferGenerateRule(expression, prefix, suffix string) (map[string]any, bool) {
	name, args, ok := parseTemplateFunction(expression)
	if !ok {
		return nil, false
	}

	rule := map[string]any{}
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "mockuuid":
		rule["kind"] = "uuid"
	case "mocknow":
		rule["kind"] = "timestamp"
		if len(args) > 0 {
			if layout := trimTemplateLiteral(args[0]); layout != "" {
				rule["layout"] = layout
			}
		}
	case "mockint":
		rule["kind"] = "int"
		if len(args) >= 1 {
			if value, ok := parseTemplateInt(args[0]); ok {
				rule["min"] = value
			}
		}
		if len(args) >= 2 {
			if value, ok := parseTemplateInt(args[1]); ok {
				rule["max"] = value
			}
		}
	case "mockstring":
		rule["kind"] = "string"
		if len(args) >= 1 {
			if value, ok := parseTemplateInt(args[0]); ok {
				rule["length"] = value
			}
		}
	case "mockpick":
		rule["kind"] = "pick"
		options := make([]any, 0, len(args))
		for _, arg := range args {
			options = append(options, trimTemplateLiteral(arg))
		}
		rule["options"] = options
	case "mockbool":
		rule["kind"] = "boolean"
	default:
		return nil, false
	}

	if prefix != "" {
		rule["prefix"] = prefix
	}
	if suffix != "" {
		rule["suffix"] = suffix
	}
	return rule, true
}

func inferResolveRule(expression, prefix, suffix string) (map[string]any, bool) {
	options := splitTemplateExpression(expression, "||")
	if len(options) == 0 || len(options) > 2 {
		return nil, false
	}

	path := strings.TrimSpace(options[0])
	if !isContextReference(path) {
		return nil, false
	}

	rule := map[string]any{
		"path": path,
	}
	if len(options) == 2 {
		fallback := trimTemplateLiteral(options[1])
		if fallback == "" && strings.TrimSpace(options[1]) != `""` && strings.TrimSpace(options[1]) != `''` {
			return nil, false
		}
		rule["fallback"] = fallback
	}
	if prefix != "" {
		rule["prefix"] = prefix
	}
	if suffix != "" {
		rule["suffix"] = suffix
	}
	return rule, true
}

func parseTemplateFunction(expression string) (string, []string, bool) {
	trimmed := strings.TrimSpace(expression)
	open := strings.IndexByte(trimmed, '(')
	close := strings.LastIndexByte(trimmed, ')')
	if open <= 0 || close != len(trimmed)-1 {
		return "", nil, false
	}
	return strings.TrimSpace(trimmed[:open]), splitTemplateExpression(trimmed[open+1:close], ","), true
}

func splitTemplateExpression(input, delimiter string) []string {
	if strings.TrimSpace(input) == "" {
		return nil
	}

	parts := make([]string, 0)
	depth := 0
	inSingleQuote := false
	inDoubleQuote := false
	start := 0

	for index := 0; index < len(input); index++ {
		char := input[index]
		switch char {
		case '\'':
			if !inDoubleQuote {
				inSingleQuote = !inSingleQuote
			}
		case '"':
			if !inSingleQuote {
				inDoubleQuote = !inDoubleQuote
			}
		case '(':
			if !inSingleQuote && !inDoubleQuote {
				depth++
			}
		case ')':
			if !inSingleQuote && !inDoubleQuote && depth > 0 {
				depth--
			}
		}

		if inSingleQuote || inDoubleQuote || depth > 0 {
			continue
		}
		if strings.HasPrefix(input[index:], delimiter) {
			parts = append(parts, strings.TrimSpace(input[start:index]))
			index += len(delimiter) - 1
			start = index + 1
		}
	}

	parts = append(parts, strings.TrimSpace(input[start:]))
	return compactTemplateParts(parts)
}

func compactTemplateParts(input []string) []string {
	output := make([]string, 0, len(input))
	for _, value := range input {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			output = append(output, trimmed)
		}
	}
	return output
}

func trimTemplateLiteral(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) >= 2 {
		if (trimmed[0] == '\'' && trimmed[len(trimmed)-1] == '\'') || (trimmed[0] == '"' && trimmed[len(trimmed)-1] == '"') {
			return trimmed[1 : len(trimmed)-1]
		}
	}
	return trimmed
}

func parseTemplateInt(value string) (int, bool) {
	parsed, err := strconv.Atoi(trimTemplateLiteral(value))
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func isContextReference(value string) bool {
	trimmed := strings.TrimSpace(value)
	switch {
	case strings.HasPrefix(trimmed, "request."):
		return true
	case strings.HasPrefix(trimmed, "state."):
		return true
	case strings.HasPrefix(trimmed, "suite."):
		return true
	case strings.HasPrefix(trimmed, "operation."):
		return true
	case strings.HasPrefix(trimmed, "match."):
		return true
	case strings.HasPrefix(trimmed, "response."):
		return true
	default:
		return false
	}
}
