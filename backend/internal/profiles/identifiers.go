package profiles

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

func uniqueProfileID(records []Record, fileName string) string {
	baseID := profileIDFromFileName(fileName)
	candidate := baseID
	index := 2
	for indexOfProfile(records, candidate) != -1 {
		candidate = fmt.Sprintf("%s-%d", baseID, index)
		index++
	}
	if candidate == "" {
		return "profile-" + uuid.NewString()[:8]
	}
	return candidate
}

func profileIDFromFileName(fileName string) string {
	trimmed := strings.TrimSpace(strings.ToLower(fileName))
	trimmed = strings.TrimSuffix(trimmed, ".yaml")
	trimmed = strings.TrimSuffix(trimmed, ".yml")
	trimmed = nonAlphaNumeric.ReplaceAllString(trimmed, "-")
	return strings.Trim(trimmed, "-")
}

func scopeFromFileName(fileName string) string {
	normalized := strings.ToLower(strings.TrimSpace(fileName))
	switch {
	case strings.Contains(normalized, "base"):
		return "Base"
	case strings.Contains(normalized, "local"):
		return "Local"
	case strings.Contains(normalized, "ci"):
		return "CI"
	case strings.Contains(normalized, "perf"):
		return "Performance"
	case strings.Contains(normalized, "stage"):
		return "Staging"
	case strings.Contains(normalized, "canary"):
		return "Canary"
	case strings.Contains(normalized, "year"):
		return "Year End"
	default:
		return labelFromFileName(fileName)
	}
}

func labelFromFileName(fileName string) string {
	trimmed := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(fileName), ".yaml"), ".yml")
	parts := strings.FieldsFunc(trimmed, func(value rune) bool {
		return value == '-' || value == '_' || value == '.'
	})
	for index, part := range parts {
		if part == "" {
			continue
		}
		parts[index] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
	}
	return strings.Join(parts, " ")
}

func indexOfProfile(records []Record, profileID string) int {
	for index, record := range records {
		if record.ID == strings.TrimSpace(profileID) {
			return index
		}
	}
	return -1
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
