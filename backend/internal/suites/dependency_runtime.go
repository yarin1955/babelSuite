package suites

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type dependencyLockDocument struct {
	Locks map[string]dependencyLockEntry `yaml:"locks"`
}

type dependencyLockEntry struct {
	Ref      string            `yaml:"ref,omitempty"`
	Version  string            `yaml:"version,omitempty"`
	Resolved string            `yaml:"resolved,omitempty"`
	Digest   string            `yaml:"digest,omitempty"`
	Profile  string            `yaml:"profile,omitempty"`
	Inputs   map[string]string `yaml:"inputs,omitempty"`
}

func (e *dependencyLockEntry) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		var resolved string
		if err := value.Decode(&resolved); err != nil {
			return err
		}
		e.Resolved = strings.TrimSpace(resolved)
		e.Digest = dependencyDigest(e.Resolved)
		return nil
	case yaml.MappingNode:
		type raw dependencyLockEntry
		var decoded raw
		if err := value.Decode(&decoded); err != nil {
			return err
		}
		*e = dependencyLockEntry(decoded)
		e.Ref = strings.TrimSpace(e.Ref)
		e.Version = strings.TrimSpace(e.Version)
		e.Resolved = strings.TrimSpace(e.Resolved)
		e.Digest = strings.TrimSpace(e.Digest)
		e.Profile = strings.TrimSpace(e.Profile)
		if e.Digest == "" {
			e.Digest = dependencyDigest(e.Resolved)
		}
		return nil
	default:
		return fmt.Errorf("dependency lock entry must be a string or object")
	}
}

type dependencyProfileRuntime struct {
	Env      map[string]string
	Services map[string]map[string]string
}

func cloneResolvedTopology(input resolvedTopology) resolvedTopology {
	return resolvedTopology{
		Nodes:        cloneTopology(input.Nodes),
		Dependencies: cloneResolvedDependencies(input.Dependencies),
	}
}

func parseDependencyLockManifest(sourceFiles []SourceFile) (map[string]dependencyLockEntry, error) {
	for _, file := range sourceFiles {
		path := strings.TrimSpace(strings.ToLower(file.Path))
		if path != "dependencies.lock.yaml" && path != "dependencies.lock.yml" {
			continue
		}

		var document dependencyLockDocument
		if err := yaml.Unmarshal([]byte(file.Content), &document); err != nil {
			return nil, fmt.Errorf("invalid suite topology: could not parse %s: %w", file.Path, err)
		}

		locks := make(map[string]dependencyLockEntry, len(document.Locks))
		for alias, entry := range document.Locks {
			trimmedAlias := strings.TrimSpace(alias)
			if trimmedAlias == "" {
				continue
			}
			locks[trimmedAlias] = entry
		}
		return locks, nil
	}

	return map[string]dependencyLockEntry{}, nil
}

func parseDependencyProfileRuntime(sourceFiles []SourceFile, fileName string) (dependencyProfileRuntime, error) {
	fileName = strings.TrimSpace(fileName)
	if fileName == "" {
		return dependencyProfileRuntime{}, nil
	}

	path := "profiles/" + strings.Trim(strings.ReplaceAll(fileName, "\\", "/"), "/")
	for _, file := range sourceFiles {
		if strings.TrimSpace(file.Path) != path {
			continue
		}

		var document map[string]any
		if err := yaml.Unmarshal([]byte(file.Content), &document); err != nil {
			return dependencyProfileRuntime{}, fmt.Errorf("invalid suite topology: could not parse %s: %w", file.Path, err)
		}

		runtime := dependencyProfileRuntime{
			Env:      scalarStringMap(document["env"]),
			Services: map[string]map[string]string{},
		}

		if services, ok := document["services"].(map[string]any); ok {
			for name, rawService := range services {
				serviceMap, ok := rawService.(map[string]any)
				if !ok {
					continue
				}
				env := scalarStringMap(serviceMap["env"])
				if len(env) == 0 {
					continue
				}
				runtime.Services[strings.TrimSpace(name)] = env
			}
		}

		return runtime, nil
	}

	return dependencyProfileRuntime{}, nil
}

func scalarStringMap(value any) map[string]string {
	items, ok := value.(map[string]any)
	if !ok || len(items) == 0 {
		return nil
	}

	out := make(map[string]string, len(items))
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		switch typed := items[key].(type) {
		case nil:
			continue
		case string:
			out[trimmedKey] = typed
		default:
			out[trimmedKey] = fmt.Sprint(typed)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func dependencyDigest(value string) string {
	_, suffix := splitDependencyRef(value)
	if strings.HasPrefix(strings.TrimSpace(suffix), "sha256:") {
		return strings.TrimSpace(suffix)
	}
	return ""
}

func dependencyHasPinnedDigest(value string) bool {
	return dependencyDigest(value) != ""
}

func dependencyUsesLatest(value string) bool {
	_, suffix := splitDependencyRef(value)
	return strings.EqualFold(strings.TrimSpace(suffix), "latest")
}

func mergeStringMaps(base map[string]string, overlays ...map[string]string) map[string]string {
	size := len(base)
	for _, overlay := range overlays {
		size += len(overlay)
	}
	if size == 0 {
		return nil
	}

	out := make(map[string]string, size)
	for key, value := range base {
		out[key] = value
	}
	for _, overlay := range overlays {
		for key, value := range overlay {
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func resolveDependencyProfile(suite Definition, requested string) (string, error) {
	profile := strings.TrimSpace(requested)
	if profile == "" {
		return defaultLaunchProfile(suite.Profiles), nil
	}
	if !suiteHasProfileOption(suite.Profiles, profile) {
		return "", fmt.Errorf("invalid suite topology: dependency %q profile does not exist in suite %q", profile, suite.ID)
	}
	return profile, nil
}

func buildDependencyRuntimeEnv(node TopologyNode, suite Definition, dependency ResolvedDependency, runtime dependencyProfileRuntime) map[string]string {
	serviceEnv := runtime.Services[node.ID]
	if len(serviceEnv) == 0 {
		serviceEnv = runtime.Services[node.Name]
	}

	env := mergeStringMaps(runtime.Env, serviceEnv, dependency.Inputs)
	env = mergeStringMaps(env, map[string]string{
		"BABELSUITE_DEPENDENCY_ALIAS": dependency.Alias,
		"BABELSUITE_DEPENDENCY_SUITE": suite.ID,
		"BABELSUITE_DEPENDENCY_REF":   dependency.Ref,
		"BABELSUITE_PROFILE":          dependency.Profile,
	})
	if dependency.Resolved != "" {
		env["BABELSUITE_DEPENDENCY_RESOLVED"] = dependency.Resolved
	}
	if dependency.Digest != "" {
		env["BABELSUITE_DEPENDENCY_DIGEST"] = dependency.Digest
	}
	return env
}

func buildDependencyRuntimeHeaders(dependency ResolvedDependency) map[string]string {
	if strings.TrimSpace(dependency.Profile) == "" {
		return nil
	}
	return map[string]string{
		"x-suite-profile": dependency.Profile,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func suiteHasProfileOption(profiles []ProfileOption, fileName string) bool {
	for _, profile := range profiles {
		if strings.TrimSpace(profile.FileName) == strings.TrimSpace(fileName) {
			return true
		}
	}
	return false
}

func defaultLaunchProfile(profiles []ProfileOption) string {
	for _, profile := range profiles {
		if profile.Default {
			return strings.TrimSpace(profile.FileName)
		}
	}
	if len(profiles) == 0 {
		return ""
	}
	return strings.TrimSpace(profiles[0].FileName)
}
