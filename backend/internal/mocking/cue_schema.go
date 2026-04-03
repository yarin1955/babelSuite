package mocking

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/babelsuite/babelsuite/internal/suites"
)

var cueSchemaContext = cuecontext.New()

func parseSchemaExamplesDocument(content string) ([]schemaBackedExample, error) {
	normalized := normalizeSchemaDocumentContent(content)

	var document schemaExampleDocument
	if err := json.Unmarshal([]byte(normalized), &document); err == nil && len(document.Examples) > 0 {
		return flattenSchemaDocument(document), nil
	}

	return parseSchemaExamplesFromCUE(normalized)
}

func flattenSchemaDocument(document schemaExampleDocument) []schemaBackedExample {
	names := make([]string, 0, len(document.Examples))
	for name := range document.Examples {
		names = append(names, name)
	}
	sort.Strings(names)

	output := make([]schemaBackedExample, 0, len(names))
	for _, name := range names {
		example := document.Examples[name]
		output = append(output, schemaBackedExample{
			Name:     name,
			Dispatch: example.Dispatch,
			Request:  example.RequestSchema,
			Response: example.ResponseSchema,
		})
	}
	return output
}

func parseSchemaExamplesFromCUE(content string) ([]schemaBackedExample, error) {
	value := cueSchemaContext.CompileString(content)
	if err := value.Err(); err != nil {
		return nil, err
	}

	examplesValue := value.LookupPath(cue.ParsePath("examples"))
	if !examplesValue.Exists() {
		return nil, fmt.Errorf("examples field not found")
	}

	iter, err := examplesValue.Fields(cue.Attributes(true))
	if err != nil {
		return nil, err
	}

	output := make([]schemaBackedExample, 0)
	for iter.Next() {
		name := cueSelectorLabel(iter.Selector())
		entry, err := parseSchemaExampleEntryValue(iter.Value())
		if err != nil {
			return nil, err
		}
		entry.Name = name
		output = append(output, entry)
	}

	sort.Slice(output, func(i, j int) bool {
		return output[i].Name < output[j].Name
	})
	return output, nil
}

func parseSchemaExampleEntryValue(value cue.Value) (schemaBackedExample, error) {
	entry := schemaBackedExample{}

	dispatch, err := parseMatchConditionsValue(value.LookupPath(cue.ParsePath("dispatch")))
	if err != nil {
		return schemaBackedExample{}, err
	}
	entry.Dispatch = dispatch

	requestValue := value.LookupPath(cue.ParsePath("requestSchema"))
	entry.Request.Headers, err = parseSchemaFieldValue(requestValue.LookupPath(cue.ParsePath("headers")), true)
	if err != nil {
		return schemaBackedExample{}, err
	}
	entry.Request.Body, err = parseSchemaFieldValue(requestValue.LookupPath(cue.ParsePath("body")), false)
	if err != nil {
		return schemaBackedExample{}, err
	}

	responseValue := value.LookupPath(cue.ParsePath("responseSchema"))
	entry.Response.Status = cueStringOrDefault(responseValue.LookupPath(cue.ParsePath("status")))
	entry.Response.MediaType = cueStringOrDefault(responseValue.LookupPath(cue.ParsePath("mediaType")))
	entry.Response.Headers, err = parseSchemaFieldValue(responseValue.LookupPath(cue.ParsePath("headers")), true)
	if err != nil {
		return schemaBackedExample{}, err
	}
	entry.Response.Body, err = parseSchemaFieldValue(responseValue.LookupPath(cue.ParsePath("body")), false)
	if err != nil {
		return schemaBackedExample{}, err
	}

	return entry, nil
}

func parseMatchConditionsValue(value cue.Value) ([]suites.MatchCondition, error) {
	if !value.Exists() {
		return nil, nil
	}

	list, err := value.List()
	if err != nil {
		return nil, nil
	}

	output := make([]suites.MatchCondition, 0)
	for list.Next() {
		item := list.Value()
		output = append(output, suites.MatchCondition{
			From:  cueStringOrDefault(item.LookupPath(cue.ParsePath("from"))),
			Param: cueStringOrDefault(item.LookupPath(cue.ParsePath("param"))),
			Value: cueStringOrDefault(item.LookupPath(cue.ParsePath("value"))),
		})
	}
	return output, nil
}

func parseSchemaFieldValue(value cue.Value, headersRequired bool) (any, error) {
	if !value.Exists() {
		return nil, nil
	}

	if attr := value.Attribute("compose"); attr.Err() == nil {
		parts, err := parseComposeAttribute(attr)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"type":            cueKindToSchemaType(value.IncompleteKind()),
			"x-babel-compose": parts,
		}, nil
	}
	if attr := value.Attribute("gen"); attr.Err() == nil {
		generate, err := parseDirectiveAttribute(attr)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"type":             cueKindToSchemaType(value.IncompleteKind()),
			"x-babel-generate": generate,
		}, nil
	}
	if attr := value.Attribute("resolve"); attr.Err() == nil {
		resolve, err := parseDirectiveAttribute(attr)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"type":            cueKindToSchemaType(value.IncompleteKind()),
			"x-babel-resolve": resolve,
		}, nil
	}
	if attr := value.Attribute("template"); attr.Err() == nil {
		template, err := attr.String(0)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"type":             cueKindToSchemaType(value.IncompleteKind()),
			"x-babel-template": template,
		}, nil
	}

	switch kind := value.IncompleteKind(); {
	case kind&cue.StructKind != 0:
		iter, err := value.Fields(cue.Attributes(true))
		if err != nil {
			return nil, err
		}
		properties := make(map[string]any)
		required := make([]string, 0)
		for iter.Next() {
			label := cueSelectorLabel(iter.Selector())
			fieldSchema, err := parseSchemaFieldValue(iter.Value(), false)
			if err != nil {
				return nil, err
			}
			properties[label] = fieldSchema
			if headersRequired {
				required = append(required, label)
			}
		}
		schema := map[string]any{
			"type":       "object",
			"properties": properties,
		}
		if headersRequired && len(required) > 0 {
			sort.Strings(required)
			schema["required"] = required
		}
		return schema, nil
	case kind&cue.ListKind != 0:
		list, err := value.List()
		if err != nil {
			return map[string]any{"type": "array"}, nil
		}
		schema := map[string]any{"type": "array"}
		if list.Next() {
			itemSchema, err := parseSchemaFieldValue(list.Value(), false)
			if err != nil {
				return nil, err
			}
			schema["items"] = itemSchema
		}
		return schema, nil
	case kind&cue.StringKind != 0:
		if literal, err := value.String(); err == nil {
			return map[string]any{"type": "string", "example": literal}, nil
		}
		return map[string]any{"type": "string"}, nil
	case kind&cue.IntKind != 0:
		if literal, err := value.Int64(); err == nil {
			return map[string]any{"type": "integer", "example": literal}, nil
		}
		return map[string]any{"type": "integer"}, nil
	case kind&cue.NumberKind != 0 || kind&cue.FloatKind != 0:
		if literal, err := value.Float64(); err == nil {
			if literal == float64(int64(literal)) {
				return map[string]any{"type": "integer", "example": int64(literal)}, nil
			}
			return map[string]any{"type": "number", "example": literal}, nil
		}
		return map[string]any{"type": "number"}, nil
	case kind&cue.BoolKind != 0:
		if literal, err := value.Bool(); err == nil {
			return map[string]any{"type": "boolean", "example": literal}, nil
		}
		return map[string]any{"type": "boolean"}, nil
	case kind&cue.NullKind != 0:
		return map[string]any{"type": "null"}, nil
	default:
		return map[string]any{"type": "string"}, nil
	}
}

func parseDirectiveAttribute(attr cue.Attribute) (map[string]any, error) {
	output := make(map[string]any)
	for index := 0; index < attr.NumArgs(); index++ {
		key, value := attr.Arg(index)
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		decoded, err := decodeCueAttributeValue(value)
		if err != nil {
			return nil, err
		}
		output[key] = decoded
	}
	return output, nil
}

func parseComposeAttribute(attr cue.Attribute) ([]any, error) {
	parts := make([]any, 0, attr.NumArgs())
	for index := 0; index < attr.NumArgs(); index++ {
		raw := strings.TrimSpace(attr.RawArg(index))
		switch {
		case strings.HasPrefix(raw, "gen("):
			spec, err := parseInlineDirective(raw, "gen")
			if err != nil {
				return nil, err
			}
			parts = append(parts, map[string]any{"generate": spec})
		case strings.HasPrefix(raw, "resolve("):
			spec, err := parseInlineDirective(raw, "resolve")
			if err != nil {
				return nil, err
			}
			parts = append(parts, map[string]any{"resolve": spec})
		default:
			decoded, err := decodeCueAttributeValue(raw)
			if err != nil {
				return nil, err
			}
			parts = append(parts, fmt.Sprint(decoded))
		}
	}
	return parts, nil
}

func parseInlineDirective(raw, name string) (map[string]any, error) {
	trimmed := strings.TrimSpace(raw)
	open := strings.IndexByte(trimmed, '(')
	close := strings.LastIndexByte(trimmed, ')')
	if open <= 0 || close != len(trimmed)-1 || strings.TrimSpace(trimmed[:open]) != name {
		return nil, fmt.Errorf("invalid directive %q", raw)
	}

	args := splitCueArguments(trimmed[open+1 : close])
	output := make(map[string]any)
	for _, arg := range args {
		key, value, found := strings.Cut(arg, "=")
		if !found {
			return nil, fmt.Errorf("invalid directive argument %q", arg)
		}
		decoded, err := decodeCueAttributeValue(value)
		if err != nil {
			return nil, err
		}
		output[strings.TrimSpace(key)] = decoded
	}
	return output, nil
}

func splitCueArguments(input string) []string {
	if strings.TrimSpace(input) == "" {
		return nil
	}

	parts := make([]string, 0)
	start := 0
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	inSingleQuote := false
	inDoubleQuote := false

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
				parenDepth++
			}
		case ')':
			if !inSingleQuote && !inDoubleQuote && parenDepth > 0 {
				parenDepth--
			}
		case '[':
			if !inSingleQuote && !inDoubleQuote {
				bracketDepth++
			}
		case ']':
			if !inSingleQuote && !inDoubleQuote && bracketDepth > 0 {
				bracketDepth--
			}
		case '{':
			if !inSingleQuote && !inDoubleQuote {
				braceDepth++
			}
		case '}':
			if !inSingleQuote && !inDoubleQuote && braceDepth > 0 {
				braceDepth--
			}
		}

		if inSingleQuote || inDoubleQuote || parenDepth > 0 || bracketDepth > 0 || braceDepth > 0 {
			continue
		}
		if char == ',' {
			parts = append(parts, strings.TrimSpace(input[start:index]))
			start = index + 1
		}
	}

	parts = append(parts, strings.TrimSpace(input[start:]))
	return compactTemplateParts(parts)
}

func decodeCueAttributeValue(raw string) (any, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}

	switch trimmed {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "null":
		return nil, nil
	}
	if integer, err := strconv.Atoi(trimmed); err == nil {
		return integer, nil
	}
	if strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, `"`) {
		value := cueSchemaContext.CompileString("x: " + trimmed).LookupPath(cue.ParsePath("x"))
		if err := value.Err(); err != nil {
			return nil, err
		}
		if value.IncompleteKind()&cue.ListKind != 0 {
			list, err := value.List()
			if err != nil {
				return nil, err
			}
			output := make([]any, 0)
			for list.Next() {
				item := list.Value()
				switch kind := item.IncompleteKind(); {
				case kind&cue.StringKind != 0:
					literal, err := item.String()
					if err != nil {
						return nil, err
					}
					output = append(output, literal)
				case kind&cue.IntKind != 0:
					literal, err := item.Int64()
					if err != nil {
						return nil, err
					}
					output = append(output, int(literal))
				default:
					body, err := item.MarshalJSON()
					if err != nil {
						return nil, err
					}
					var decoded any
					if err := json.Unmarshal(body, &decoded); err != nil {
						return nil, err
					}
					output = append(output, decoded)
				}
			}
			return output, nil
		}
		if literal, err := value.String(); err == nil {
			return literal, nil
		}
		body, err := value.MarshalJSON()
		if err != nil {
			return nil, err
		}
		var decoded any
		if err := json.Unmarshal(body, &decoded); err != nil {
			return nil, err
		}
		return decoded, nil
	}
	return strings.Trim(trimmed, `"'`), nil
}

func cueKindToSchemaType(kind cue.Kind) string {
	switch {
	case kind&cue.IntKind != 0:
		return "integer"
	case kind&cue.NumberKind != 0 || kind&cue.FloatKind != 0:
		return "number"
	case kind&cue.BoolKind != 0:
		return "boolean"
	case kind&cue.NullKind != 0:
		return "null"
	default:
		return "string"
	}
}

func cueStringOrDefault(value cue.Value) string {
	if !value.Exists() {
		return ""
	}
	if literal, err := value.String(); err == nil {
		return literal
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func cueSelectorLabel(selector cue.Selector) string {
	if selector.IsString() {
		return selector.Unquoted()
	}
	return selector.String()
}
