package mocking

import (
	cryptoRand "crypto/rand"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/babelsuite/babelsuite/internal/suites"
)

func buildTemplateContext(suite suites.Definition, surface suites.APISurface, operation suites.APIOperation, snapshot requestSnapshot, state map[string]string, responseBody map[string]any, exampleName string) map[string]any {
	return map[string]any{
		"suite": map[string]any{
			"id":           suite.ID,
			"title":        suite.Title,
			"repository":   suite.Repository,
			"surfaceId":    surface.ID,
			"surfaceTitle": surface.Title,
		},
		"operation": map[string]any{
			"id":          operation.ID,
			"name":        operation.Name,
			"resolverUrl": operation.MockMetadata.ResolverURL,
			"runtimeUrl":  operation.MockMetadata.RuntimeURL,
		},
		"request": map[string]any{
			"method":  snapshot.Method,
			"query":   snapshot.Query,
			"path":    snapshot.Path,
			"headers": snapshot.Headers,
			"body":    firstNonNil(snapshot.BodyJSON, snapshot.BodyObject),
			"rawBody": snapshot.BodyRaw,
		},
		"response": map[string]any{
			"body": responseBody,
		},
		"state": firstNonNil(state, map[string]string{}),
		"match": map[string]any{"example": exampleName},
	}
}

func renderTemplate(input string, contextMap map[string]any) string {
	if strings.TrimSpace(input) == "" {
		return input
	}

	return templateTokenPattern.ReplaceAllStringFunc(input, func(token string) string {
		match := templateTokenPattern.FindStringSubmatch(token)
		if len(match) != 2 {
			return token
		}
		value, ok := evaluateTemplateExpression(match[1], contextMap)
		if !ok {
			return ""
		}
		return value
	})
}

func evaluateTemplateExpression(expression string, contextMap map[string]any) (string, bool) {
	options := splitTemplateExpression(expression, "||")
	if len(options) > 1 {
		for _, option := range options {
			value, ok := evaluateTemplateExpression(option, contextMap)
			if ok && strings.TrimSpace(value) != "" {
				return value, true
			}
		}
		return "", false
	}

	trimmed := strings.TrimSpace(expression)
	if trimmed == "" {
		return "", false
	}

	if literal, ok := unquoteTemplateLiteral(trimmed); ok {
		return literal, true
	}

	value, ok := lookupContextValue(contextMap, templatePathSegments(trimmed))
	if !ok {
		return "", false
	}
	return fmt.Sprint(value), true
}

func lookupContextValue(value any, segments []string) (any, bool) {
	current := value
	for _, segment := range segments {
		switch typed := current.(type) {
		case map[string]any:
			next, ok := typed[segment]
			if !ok {
				return nil, false
			}
			current = next
		case map[string]string:
			next, ok := typed[segment]
			if !ok {
				return nil, false
			}
			current = next
		default:
			return nil, false
		}
	}
	return current, true
}

func unquoteTemplateLiteral(value string) (string, bool) {
	if len(value) < 2 {
		return "", false
	}
	if (value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"') {
		unquoted, err := strconv.Unquote(`"` + strings.ReplaceAll(value[1:len(value)-1], `"`, `\"`) + `"`)
		if err != nil {
			return value[1 : len(value)-1], true
		}
		return unquoted, true
	}
	return "", false
}

func timeLayoutForTemplateArg(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.RFC3339Nano
	}

	replacements := []struct {
		from string
		to   string
	}{
		{"yyyy", "2006"},
		{"MM", "01"},
		{"dd", "02"},
		{"HH", "15"},
		{"mm", "04"},
		{"ss", "05"},
	}

	layout := trimmed
	for _, replacement := range replacements {
		layout = strings.ReplaceAll(layout, replacement.from, replacement.to)
	}
	return layout
}

func generateIntInRange(minimum, maximum int) int {
	if maximum < minimum {
		minimum, maximum = maximum, minimum
	}
	if maximum == minimum {
		return minimum
	}

	span := int64(maximum-minimum) + 1
	value, err := cryptoRand.Int(cryptoRand.Reader, big.NewInt(span))
	if err != nil {
		return minimum
	}
	return minimum + int(value.Int64())
}

func generateRandomString(length int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	builder := strings.Builder{}
	builder.Grow(length)
	for index := 0; index < length; index++ {
		position, err := cryptoRand.Int(cryptoRand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			builder.WriteByte(alphabet[index%len(alphabet)])
			continue
		}
		builder.WriteByte(alphabet[position.Int64()])
	}
	return builder.String()
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

func templatePathSegments(path string) []string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil
	}

	segments := make([]string, 0)
	buffer := strings.Builder{}
	for index := 0; index < len(trimmed); index++ {
		char := trimmed[index]
		switch char {
		case '.', '/':
			if buffer.Len() > 0 {
				segments = append(segments, buffer.String())
				buffer.Reset()
			}
		case '[':
			if buffer.Len() > 0 {
				segments = append(segments, buffer.String())
				buffer.Reset()
			}
			end := strings.IndexByte(trimmed[index+1:], ']')
			if end < 0 {
				continue
			}
			key := strings.TrimSpace(trimmed[index+1 : index+1+end])
			key = strings.Trim(key, `"'`)
			if key != "" {
				segments = append(segments, key)
			}
			index += end + 1
		default:
			buffer.WriteByte(char)
		}
	}
	if buffer.Len() > 0 {
		segments = append(segments, buffer.String())
	}
	return compactTemplateParts(segments)
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}
