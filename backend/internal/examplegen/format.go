package examplegen

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

func formatJSON(value any) string {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "{}\n"
	}
	return string(body) + "\n"
}

func formatMockSchemaDocument(path string, value any) string {
	if strings.EqualFold(filepath.Ext(strings.TrimSpace(path)), ".cue") {
		return formatCUE(value)
	}
	return formatJSON(value)
}
