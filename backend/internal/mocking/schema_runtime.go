package mocking

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/babelsuite/babelsuite/internal/suites"
	"github.com/google/uuid"
)

func loadSchemaExamples(suite suites.Definition, operation suites.APIOperation) []schemaBackedExample {
	content, ok := sourceFileContent(suite.SourceFiles, operation.MockPath)
	if !ok {
		return nil
	}
	examples, err := parseSchemaExamplesDocument(content)
	if err != nil {
		return nil
	}
	return examples
}

func normalizeSchemaDocumentContent(content string) string {
	if !strings.Contains(content, "//") {
		return content
	}

	lines := strings.Split(content, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.Join(filtered, "\n")
}

func sourceFileContent(files []suites.SourceFile, path string) (string, bool) {
	for _, file := range files {
		if strings.TrimSpace(file.Path) == strings.TrimSpace(path) {
			return file.Content, true
		}
	}
	return "", false
}

func matchSchemaExample(examples []schemaBackedExample, snapshot requestSnapshot) (schemaBackedExample, bool, *Result) {
	if len(examples) == 1 && len(examples[0].Dispatch) == 0 {
		if result := validateSchemaRequest(examples[0].Request, snapshot); result != nil {
			return schemaBackedExample{}, false, result
		}
		return examples[0], true, nil
	}

	for _, example := range examples {
		if !matchWhen(example.Dispatch, snapshot) {
			continue
		}
		if result := validateSchemaRequest(example.Request, snapshot); result != nil {
			return schemaBackedExample{}, false, result
		}
		return example, true, nil
	}
	return schemaBackedExample{}, false, nil
}

func validateSchemaRequest(requestSchema schemaRequestSpec, snapshot requestSnapshot) *Result {
	if message := validateSchemaValue(requestSchema.Headers, snapshot.Headers, "request headers"); message != "" {
		return errorResult(http.StatusBadRequest, "application/json", fmt.Sprintf(`{"error":"%s"}`, escapeJSONString(message)))
	}
	if message := validateSchemaValue(requestSchema.Body, snapshot.BodyJSON, "request body"); message != "" {
		return errorResult(http.StatusBadRequest, "application/json", fmt.Sprintf(`{"error":"%s"}`, escapeJSONString(message)))
	}
	return nil
}

func validateSchemaValue(schema any, value any, location string) string {
	node, ok := schema.(map[string]any)
	if !ok || len(node) == 0 {
		return ""
	}

	schemaType := strings.TrimSpace(fmt.Sprint(node["type"]))
	switch schemaType {
	case "null":
		if value != nil {
			return fmt.Sprintf("%s must be empty.", strings.Title(location))
		}
	case "object":
		object, ok := toMapAny(value)
		if !ok {
			return fmt.Sprintf("%s must be an object.", strings.Title(location))
		}
		required := schemaStringList(node["required"])
		properties, _ := node["properties"].(map[string]any)
		for _, key := range required {
			actual, exists := object[key]
			if !exists || actual == nil || (strings.TrimSpace(fmt.Sprint(actual)) == "" && !isZeroBoolean(actual) && !isNumericValue(actual)) {
				return fmt.Sprintf("Missing required %s field %q.", location, key)
			}
		}
		for key, propertySchema := range properties {
			actual, exists := object[key]
			if !exists {
				continue
			}
			if message := validateSchemaValue(propertySchema, actual, location+"."+key); message != "" {
				return message
			}
		}
	case "array":
		items, ok := value.([]any)
		if !ok {
			if value == nil {
				return ""
			}
			return fmt.Sprintf("%s must be an array.", strings.Title(location))
		}
		itemSchema, _ := node["items"]
		for index, item := range items {
			if message := validateSchemaValue(itemSchema, item, fmt.Sprintf("%s[%d]", location, index)); message != "" {
				return message
			}
		}
	case "string":
		if value == nil {
			return ""
		}
		if _, ok := value.(string); !ok {
			return fmt.Sprintf("%s must be a string.", strings.Title(location))
		}
	case "integer":
		if value == nil {
			return ""
		}
		if !isIntegerValue(value) {
			return fmt.Sprintf("%s must be an integer.", strings.Title(location))
		}
	case "number":
		if value == nil {
			return ""
		}
		if !isNumericValue(value) {
			return fmt.Sprintf("%s must be a number.", strings.Title(location))
		}
	case "boolean":
		if value == nil {
			return ""
		}
		if _, ok := value.(bool); !ok {
			return fmt.Sprintf("%s must be a boolean.", strings.Title(location))
		}
	}

	return ""
}

func buildSchemaResult(suite suites.Definition, surface suites.APISurface, operation suites.APIOperation, snapshot requestSnapshot, state map[string]string, adapter string, example schemaBackedExample) *Result {
	contextMap := buildTemplateContext(suite, surface, operation, snapshot, state, nil, example.Name)
	headers := renderSchemaHeaders(example.Response.Headers, contextMap)
	applyRecopiedConstraintHeaders(headers, operation.MockMetadata.ParameterConstraints, snapshot)
	body := renderSchemaBody(example.Response.Body, example.Response.MediaType, contextMap)
	return &Result{
		Status:         parseStatusCode(example.Response.Status, http.StatusOK),
		MediaType:      firstNonEmpty(example.Response.MediaType, inferredMediaType(body)),
		Headers:        headers,
		Body:           []byte(body),
		Adapter:        adapter,
		Dispatcher:     firstNonEmpty(operation.Dispatcher, operation.MockMetadata.Dispatcher),
		ResolverURL:    operation.MockMetadata.ResolverURL,
		RuntimeURL:     operation.MockMetadata.RuntimeURL,
		MatchedExample: example.Name,
	}
}

func renderSchemaHeaders(schema any, contextMap map[string]any) http.Header {
	headers := make(http.Header)
	values, ok := renderSchemaValue(schema, contextMap).(map[string]any)
	if !ok {
		return headers
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if values[key] == nil {
			continue
		}
		headers.Set(key, fmt.Sprint(values[key]))
	}
	return headers
}

func renderSchemaBody(schema any, mediaType string, contextMap map[string]any) string {
	rendered := renderSchemaValue(schema, contextMap)
	if rendered == nil {
		return ""
	}

	normalizedMediaType := strings.ToLower(strings.TrimSpace(mediaType))
	switch rendered.(type) {
	case map[string]any, []any:
		body, err := json.MarshalIndent(rendered, "", "  ")
		if err == nil {
			return string(body)
		}
	}
	if normalizedMediaType == "" {
		normalizedMediaType = inferredMediaType(fmt.Sprint(rendered))
	}
	if strings.Contains(normalizedMediaType, "json") {
		body, err := json.MarshalIndent(rendered, "", "  ")
		if err == nil {
			return string(body)
		}
	}
	return fmt.Sprint(rendered)
}

func renderSchemaValue(schema any, contextMap map[string]any) any {
	node, ok := schema.(map[string]any)
	if !ok {
		return schema
	}

	if compose, ok := node["x-babel-compose"]; ok {
		if value, ok := renderSchemaComposedValue(compose, contextMap); ok {
			return coerceSchemaAny(strings.TrimSpace(fmt.Sprint(node["type"])), value)
		}
	}
	if generate, ok := node["x-babel-generate"]; ok {
		if value, ok := renderSchemaGeneratedValue(strings.TrimSpace(fmt.Sprint(node["type"])), generate); ok {
			return value
		}
	}
	if resolve, ok := node["x-babel-resolve"]; ok {
		if value, ok := renderSchemaResolvedValue(strings.TrimSpace(fmt.Sprint(node["type"])), resolve, contextMap); ok {
			return value
		}
	}
	if template, ok := node["x-babel-template"].(string); ok {
		return coerceSchemaPrimitive(strings.TrimSpace(fmt.Sprint(node["type"])), renderTemplate(template, contextMap))
	}
	if example, exists := node["example"]; exists {
		return example
	}

	switch strings.TrimSpace(fmt.Sprint(node["type"])) {
	case "object":
		properties, _ := node["properties"].(map[string]any)
		if len(properties) == 0 {
			return map[string]any{}
		}
		keys := make([]string, 0, len(properties))
		for key := range properties {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		result := make(map[string]any, len(keys))
		for _, key := range keys {
			result[key] = renderSchemaValue(properties[key], contextMap)
		}
		return result
	case "array":
		if items, exists := node["items"]; exists {
			return []any{renderSchemaValue(items, contextMap)}
		}
		return []any{}
	case "integer":
		return int64(0)
	case "number":
		return 0.0
	case "boolean":
		return false
	case "null":
		return nil
	default:
		return ""
	}
}

func renderSchemaComposedValue(spec any, contextMap map[string]any) (string, bool) {
	parts, ok := spec.([]any)
	if !ok || len(parts) == 0 {
		return "", false
	}

	builder := strings.Builder{}
	for _, part := range parts {
		switch typed := part.(type) {
		case string:
			builder.WriteString(typed)
		case map[string]any:
			if generate, exists := typed["generate"]; exists {
				value, ok := renderSchemaGeneratedValue("string", generate)
				if !ok {
					return "", false
				}
				builder.WriteString(fmt.Sprint(value))
				continue
			}
			if resolve, exists := typed["resolve"]; exists {
				value, ok := renderSchemaResolvedValue("string", resolve, contextMap)
				if !ok {
					return "", false
				}
				builder.WriteString(fmt.Sprint(value))
				continue
			}
			return "", false
		default:
			return "", false
		}
	}

	return builder.String(), true
}

func renderSchemaGeneratedValue(schemaType string, spec any) (any, bool) {
	node, ok := spec.(map[string]any)
	if !ok {
		return nil, false
	}

	kind := strings.ToLower(strings.TrimSpace(stringValue(node["kind"])))
	prefix := stringValue(node["prefix"])
	suffix := stringValue(node["suffix"])
	layout := stringValue(node["layout"])

	var value any
	switch kind {
	case "uuid":
		value = uuid.NewString()
	case "timestamp":
		if strings.TrimSpace(layout) == "" {
			layout = time.RFC3339Nano
		} else {
			layout = timeLayoutForTemplateArg(layout)
		}
		value = time.Now().UTC().Format(layout)
	case "int":
		minimum := 0
		maximum := 2147483647
		if value, ok := intValue(node["min"]); ok {
			minimum = value
		}
		if value, ok := intValue(node["max"]); ok {
			maximum = value
		}
		generated := generateIntInRange(minimum, maximum)
		if prefix != "" || suffix != "" || schemaType == "string" {
			value = prefix + strconv.Itoa(generated) + suffix
		} else {
			value = generated
		}
	case "string":
		length, _ := intValue(node["length"])
		if length <= 0 {
			length = 12
		}
		value = prefix + generateRandomString(length) + suffix
	case "pick":
		options := anySlice(node["options"])
		if len(options) == 0 {
			return nil, false
		}
		index := generateIntInRange(0, len(options)-1)
		value = prefix + fmt.Sprint(options[index]) + suffix
	case "boolean":
		generated := generateIntInRange(0, 1) == 1
		if prefix != "" || suffix != "" || schemaType == "string" {
			value = prefix + strconv.FormatBool(generated) + suffix
		} else {
			value = generated
		}
	default:
		return nil, false
	}

	return coerceSchemaAny(schemaType, value), true
}

func renderSchemaResolvedValue(schemaType string, spec any, contextMap map[string]any) (any, bool) {
	node, ok := spec.(map[string]any)
	if !ok {
		return nil, false
	}

	path := strings.TrimSpace(stringValue(node["path"]))
	if path == "" {
		return nil, false
	}

	value, ok := lookupContextValue(contextMap, templatePathSegments(path))
	if !ok || value == nil || strings.TrimSpace(fmt.Sprint(value)) == "" {
		if fallback, exists := node["fallback"]; exists {
			value = fallback
			ok = true
		}
	}
	if !ok {
		return nil, false
	}

	prefix := stringValue(node["prefix"])
	suffix := stringValue(node["suffix"])
	if prefix != "" || suffix != "" {
		return coerceSchemaAny(schemaType, prefix+fmt.Sprint(value)+suffix), true
	}
	return coerceSchemaAny(schemaType, value), true
}

func coerceSchemaPrimitive(schemaType, value string) any {
	switch schemaType {
	case "integer":
		if parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil {
			return parsed
		}
	case "number":
		if parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
			return parsed
		}
	case "boolean":
		if parsed, err := strconv.ParseBool(strings.TrimSpace(value)); err == nil {
			return parsed
		}
	case "null":
		return nil
	}
	return value
}

func coerceSchemaAny(schemaType string, value any) any {
	if value == nil {
		return nil
	}
	switch typed := value.(type) {
	case string:
		return coerceSchemaPrimitive(schemaType, typed)
	default:
		switch schemaType {
		case "string":
			return fmt.Sprint(value)
		case "integer":
			if isIntegerValue(value) {
				return value
			}
		case "number":
			if isNumericValue(value) {
				return value
			}
		case "boolean":
			if _, ok := value.(bool); ok {
				return value
			}
		}
		return value
	}
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func intValue(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int8:
		return int(typed), true
	case int16:
		return int(typed), true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case float32:
		return int(typed), true
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func anySlice(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	case []string:
		output := make([]any, 0, len(typed))
		for _, item := range typed {
			output = append(output, item)
		}
		return output
	default:
		return nil
	}
}

func toMapAny(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return typed, true
	case map[string]string:
		output := make(map[string]any, len(typed))
		for key, entry := range typed {
			output[key] = entry
		}
		return output, true
	default:
		return nil, false
	}
}

func schemaStringList(value any) []string {
	input, ok := value.([]any)
	if !ok {
		return nil
	}
	output := make([]string, 0, len(input))
	for _, item := range input {
		trimmed := strings.TrimSpace(fmt.Sprint(item))
		if trimmed != "" {
			output = append(output, trimmed)
		}
	}
	return output
}

func isNumericValue(value any) bool {
	switch value.(type) {
	case int, int8, int16, int32, int64, float32, float64, uint, uint8, uint16, uint32, uint64:
		return true
	default:
		return false
	}
}

func isIntegerValue(value any) bool {
	switch typed := value.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return true
	case float64:
		return typed == float64(int64(typed))
	case float32:
		return typed == float32(int64(typed))
	default:
		return false
	}
}

func isZeroBoolean(value any) bool {
	typed, ok := value.(bool)
	return ok && !typed
}

func bodyLookup(body any, key string) string {
	if body == nil || strings.TrimSpace(key) == "" {
		return ""
	}

	current := body
	for _, segment := range strings.Split(key, ".") {
		switch typed := current.(type) {
		case map[string]any:
			current = typed[segment]
		default:
			return ""
		}
	}
	if current == nil {
		return ""
	}
	return fmt.Sprint(current)
}
