package catalog

import (
	"sort"
	"strings"
)

func chooseVersion(tags []string, preferred string) string {
	preferred = strings.TrimSpace(preferred)
	if preferred != "" {
		for _, tag := range tags {
			if tag == preferred {
				return preferred
			}
		}
	}

	for _, tag := range tags {
		if !strings.EqualFold(tag, "latest") {
			return tag
		}
	}
	if len(tags) > 0 {
		return tags[0]
	}
	if preferred != "" {
		return preferred
	}
	return "latest"
}

func sortTags(tags []string) []string {
	out := append([]string{}, tags...)
	sort.Slice(out, func(i, j int) bool {
		left := strings.ToLower(out[i])
		right := strings.ToLower(out[j])
		if left == "latest" || right == "latest" {
			return left == "latest"
		}
		return compareVersionLike(out[i], out[j]) > 0
	})
	return out
}

func compareVersionLike(left, right string) int {
	left = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(left)), "v")
	right = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(right)), "v")

	leftParts := splitVersionParts(left)
	rightParts := splitVersionParts(right)
	maxParts := len(leftParts)
	if len(rightParts) > maxParts {
		maxParts = len(rightParts)
	}

	for index := 0; index < maxParts; index++ {
		leftPart := ""
		rightPart := ""
		if index < len(leftParts) {
			leftPart = leftParts[index]
		}
		if index < len(rightParts) {
			rightPart = rightParts[index]
		}
		if leftPart == rightPart {
			continue
		}
		if isNumeric(leftPart) && isNumeric(rightPart) {
			if len(leftPart) != len(rightPart) {
				if len(leftPart) > len(rightPart) {
					return 1
				}
				return -1
			}
		}
		if leftPart > rightPart {
			return 1
		}
		return -1
	}

	return 0
}

func splitVersionParts(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		return r == '.' || r == '-' || r == '_'
	})
}

func isNumeric(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
