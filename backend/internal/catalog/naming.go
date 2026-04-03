package catalog

import (
	"fmt"
	"strings"
)

func packageID(repository, kind string) string {
	base := strings.ToLower(strings.TrimSpace(repository))
	replacer := strings.NewReplacer("/", "-", ":", "-", ".", "-", "_", "-")
	base = replacer.Replace(base)
	base = strings.Trim(base, "-")
	if base == "" {
		base = "package"
	}
	return kind + "-" + base
}

func inferKind(repository string) string {
	repository = strings.ToLower(strings.TrimSpace(repository))
	if strings.HasPrefix(repository, "babelsuite/") || strings.Contains(repository, "/babelsuite/") {
		return "stdlib"
	}
	return "suite"
}

func titleForRepository(repository, kind string) string {
	repository = strings.TrimSpace(repository)
	if kind == "stdlib" {
		name := repository
		if index := strings.Index(name, "/"); index >= 0 {
			name = name[index+1:]
		}
		return "@babelsuite/" + name
	}

	parts := strings.Split(repository, "/")
	return humanize(parts[len(parts)-1])
}

func ownerForRepository(repository, fallback string) string {
	parts := strings.Split(strings.Trim(repository, "/"), "/")
	if len(parts) == 0 {
		return firstNonEmpty(fallback, "Registry Package")
	}
	return humanize(parts[0])
}

func genericDescription(repository, registryName, kind string) string {
	if kind == "stdlib" {
		return fmt.Sprintf("Discovered in %s and treated as a BabelSuite standard library module because of its repository path.", firstNonEmpty(registryName, "the configured registry"))
	}
	return fmt.Sprintf("Discovered directly from %s. Publish richer suite metadata inside BabelSuite to unlock deep inspect, topology, and contract views.", firstNonEmpty(registryName, "the configured registry"))
}

func inferModules(repository string) []string {
	repository = strings.ToLower(strings.TrimSpace(repository))
	modules := make([]string, 0, 4)
	for _, candidate := range []string{"postgres", "kafka", "wiremock", "mock-api", "playwright", "grpc", "redis", "prometheus", "vault"} {
		if strings.Contains(repository, candidate) || (candidate == "mock-api" && strings.Contains(repository, "mock")) {
			modules = append(modules, candidate)
		}
	}
	return modules
}

func humanize(value string) string {
	parts := strings.FieldsFunc(strings.TrimSpace(value), func(r rune) bool {
		return r == '-' || r == '_' || r == '.'
	})
	if len(parts) == 0 {
		return value
	}
	for index, part := range parts {
		if part == strings.ToUpper(part) {
			parts[index] = part
			continue
		}
		parts[index] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
	}
	return strings.Join(parts, " ")
}

func buildRunCommand(repository, version string) string {
	return "babelctl run " + repository + ":" + chooseVersion(nil, version)
}

func buildForkCommand(repository, version string) string {
	name := repository
	if parts := strings.Split(strings.Trim(repository, "/"), "/"); len(parts) > 0 {
		name = parts[len(parts)-1]
	}
	return "babelctl fork " + repository + ":" + chooseVersion(nil, version) + " ./" + name
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
