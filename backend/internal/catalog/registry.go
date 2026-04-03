package catalog

import (
	"errors"
	"net/url"
	"strings"
)

func normalizeRegistryURL(raw string) (string, string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", "", errors.New("registry URL is empty")
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return "", "", err
	}
	if parsed.Host == "" {
		return "", "", errors.New("registry URL host is empty")
	}

	parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	return parsed.String(), parsed.Host, nil
}

func encodeRepositoryPath(repository string) string {
	parts := strings.Split(strings.Trim(repository, "/"), "/")
	for index := range parts {
		parts[index] = url.PathEscape(parts[index])
	}
	return strings.Join(parts, "/")
}

func matchesRepositoryScope(repository, scope string) bool {
	repository = strings.Trim(strings.ToLower(repository), "/")
	scope = strings.TrimSpace(scope)
	if scope == "" || scope == "*" {
		return true
	}

	for _, candidate := range strings.Split(scope, ",") {
		candidate = strings.Trim(strings.ToLower(candidate), "/")
		if candidate == "" || candidate == "*" {
			return true
		}
		if repository == candidate || strings.HasPrefix(repository, candidate+"/") {
			return true
		}
	}

	return false
}

func normalizeRepository(repository string) string {
	return strings.ToLower(strings.TrimSpace(repository))
}

func repositoryPath(repository string) string {
	repository = strings.TrimSpace(repository)
	if repository == "" {
		return ""
	}
	if strings.Contains(repository, "://") {
		if parsed, err := url.Parse(repository); err == nil {
			return strings.Trim(parsed.Path, "/")
		}
	}
	if slash := strings.Index(repository, "/"); slash >= 0 {
		host := repository[:slash]
		if strings.Contains(host, ".") || strings.Contains(host, ":") || strings.EqualFold(host, "localhost") {
			return strings.Trim(repository[slash+1:], "/")
		}
	}
	return strings.Trim(repository, "/")
}
