package environments

import (
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

func classifyDockerError(err error, output []byte) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, exec.ErrNotFound) {
		return fmt.Errorf("%w: docker CLI was not found in PATH", ErrDockerUnavailable)
	}

	text := strings.ToLower(string(output) + " " + err.Error())
	if strings.Contains(text, "cannot connect to the docker daemon") ||
		strings.Contains(text, "error during connect") ||
		strings.Contains(text, "daemon is not running") ||
		strings.Contains(text, "docker desktop") {
		return fmt.Errorf("%w: %s", ErrDockerUnavailable, strings.TrimSpace(string(output)))
	}

	return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
}

func humanizeDockerError(err error) string {
	if err == nil {
		return ""
	}
	return strings.TrimSpace(err.Error())
}

func formatPorts(ports map[string][]dockerPortBinding) []string {
	if len(ports) == 0 {
		return []string{}
	}

	result := []string{}
	for containerPort, bindings := range ports {
		if len(bindings) == 0 {
			result = append(result, containerPort)
			continue
		}
		for _, binding := range bindings {
			if binding.HostPort == "" {
				result = append(result, containerPort)
				continue
			}
			result = append(result, fmt.Sprintf("%s -> %s", binding.HostPort, containerPort))
		}
	}

	return sortStrings(result)
}

func parseDockerTime(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" || value == "0001-01-01T00:00:00Z" {
		return nil
	}

	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			utc := parsed.UTC()
			return &utc
		}
	}
	return nil
}

func parseInt(value string) int {
	number, _ := strconv.Atoi(strings.TrimSpace(value))
	return number
}

func parsePercent(value string) float64 {
	value = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(value), "%"))
	number, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	return roundFloat(number)
}

var sizePattern = regexp.MustCompile(`(?i)^\s*([0-9]+(?:\.[0-9]+)?)\s*([kmgtp]?i?b)\s*$`)

func parseMemoryUsage(value string) (int64, int64) {
	parts := strings.Split(value, "/")
	if len(parts) != 2 {
		return 0, 0
	}
	return parseHumanSize(parts[0]), parseHumanSize(parts[1])
}

func parseHumanSize(value string) int64 {
	match := sizePattern.FindStringSubmatch(strings.TrimSpace(value))
	if len(match) != 3 {
		return 0
	}

	number, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return 0
	}

	unit := strings.ToLower(match[2])
	multiplier := float64(1)
	switch unit {
	case "kb":
		multiplier = 1000
	case "mb":
		multiplier = 1000 * 1000
	case "gb":
		multiplier = 1000 * 1000 * 1000
	case "tb":
		multiplier = 1000 * 1000 * 1000 * 1000
	case "kib":
		multiplier = 1024
	case "mib":
		multiplier = 1024 * 1024
	case "gib":
		multiplier = 1024 * 1024 * 1024
	case "tib":
		multiplier = 1024 * 1024 * 1024 * 1024
	}

	return int64(number * multiplier)
}

func splitLines(value []byte) []string {
	lines := strings.Split(strings.ReplaceAll(string(value), "\r\n", "\n"), "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func compactStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func roundFloat(value float64) float64 {
	parsed, _ := strconv.ParseFloat(fmt.Sprintf("%.2f", value), 64)
	return parsed
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func labelIsTrue(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	return normalized == "true" || normalized == "1" || normalized == "yes"
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "unknown"
	}
	var builder strings.Builder
	lastDash := false
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') {
			builder.WriteRune(char)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	result := strings.Trim(builder.String(), "-")
	if result == "" {
		return "unknown"
	}
	return result
}

func zeroTime(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}

func cloneLabels(source map[string]string) map[string]string {
	if len(source) == 0 {
		return map[string]string{}
	}
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func sortStrings(values []string) []string {
	if len(values) <= 1 {
		return values
	}
	sort.Strings(values)
	return values
}
