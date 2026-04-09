package suites

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	topologyAssignmentPattern = regexp.MustCompile(`^([a-zA-Z_][\w]*)\s*=\s*([a-zA-Z_][\w]*(?:\.[a-zA-Z_][\w]*)?)\((.*)\)$`)
	topologyNamePattern       = regexp.MustCompile(`(?:^|,)\s*(?:name|name_or_id|id)\s*=\s*"([^"]+)"`)
	topologyAfterPattern      = regexp.MustCompile(`(?:^|,)\s*after\s*=\s*\[([^\]]*)\]`)
	topologyRefPattern        = regexp.MustCompile(`(?:^|,)\s*ref\s*=\s*"([^"]+)"`)
)

type rawTopologyNode struct {
	Assignment string
	ID         string
	Name       string
	Kind       string
	Ref        string
	DependsOn  []string
	Order      int
}

type dependencyManifest struct {
	Dependencies map[string]dependencyEntry `yaml:"dependencies"`
}

type dependencyEntry struct {
	Ref     string            `yaml:"ref"`
	Version string            `yaml:"version,omitempty"`
	Profile string            `yaml:"profile,omitempty"`
	Inputs  map[string]string `yaml:"inputs,omitempty"`
}

func (e *dependencyEntry) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		var ref string
		if err := value.Decode(&ref); err != nil {
			return err
		}
		e.Ref = strings.TrimSpace(ref)
		return nil
	case yaml.MappingNode:
		type raw dependencyEntry
		var decoded raw
		if err := value.Decode(&decoded); err != nil {
			return err
		}
		*e = dependencyEntry(decoded)
		e.Ref = strings.TrimSpace(e.Ref)
		e.Version = strings.TrimSpace(e.Version)
		e.Profile = strings.TrimSpace(e.Profile)
		return nil
	default:
		return fmt.Errorf("dependency entry must be a string or object")
	}
}

type topologyImport struct {
	EntryIDs []string
	ExitIDs  []string
	Nodes    []TopologyNode
}

type resolvedTopology struct {
	Nodes        []TopologyNode
	Dependencies []ResolvedDependency
}

func ResolveTopology(suite Definition, catalog []Definition) ([]TopologyNode, error) {
	resolved, err := ResolveRuntime(suite, catalog)
	if err != nil {
		return nil, err
	}
	return resolved.Nodes, nil
}

func ResolveRuntime(suite Definition, catalog []Definition) (resolvedTopology, error) {
	resolver := newTopologyResolver(catalog)
	return resolver.resolveSuite(suite, nil)
}

func ResolveDefinitionTopology(suite Definition, catalog []Definition) Definition {
	resolved, err := ResolveRuntime(suite, catalog)
	if err != nil {
		suite.Topology = nil
		suite.ResolvedDependencies = nil
		suite.TopologyError = err.Error()
		return suite
	}
	suite.Topology = resolved.Nodes
	suite.ResolvedDependencies = resolved.Dependencies
	suite.TopologyError = ""
	return suite
}

type topologyResolver struct {
	byID   map[string]Definition
	cached map[string]resolvedTopology
}

func newTopologyResolver(catalog []Definition) *topologyResolver {
	byID := make(map[string]Definition, len(catalog))
	for _, suite := range catalog {
		byID[strings.TrimSpace(suite.ID)] = suite
	}
	return &topologyResolver{
		byID:   byID,
		cached: make(map[string]resolvedTopology),
	}
}

func (r *topologyResolver) resolveSuite(suite Definition, stack []string) (resolvedTopology, error) {
	id := strings.TrimSpace(suite.ID)
	if cached, ok := r.cached[id]; ok {
		return cloneResolvedTopology(cached), nil
	}
	if err := ValidateDefinition(suite); err != nil {
		return resolvedTopology{}, fmt.Errorf("invalid suite package: %w", err)
	}
	if containsString(stack, id) {
		path := append(append([]string{}, stack...), id)
		return resolvedTopology{}, fmt.Errorf("invalid suite topology: nested suite cycle detected: %s", strings.Join(path, " -> "))
	}

	rawNodes, err := parseRawTopology(suite.SuiteStar)
	if err != nil {
		return resolvedTopology{}, err
	}
	manifest, err := parseDependencyManifest(suite.SourceFiles)
	if err != nil {
		return resolvedTopology{}, err
	}
	locks, err := parseDependencyLockManifest(suite.SourceFiles)
	if err != nil {
		return resolvedTopology{}, err
	}

	imports := make(map[string]topologyImport)
	dependencies := make([]ResolvedDependency, 0)
	nextStack := append(append([]string{}, stack...), id)
	for _, raw := range rawNodes {
		if raw.Kind != "suite" {
			continue
		}

		entry, ok := manifest[raw.Ref]
		if !ok {
			return resolvedTopology{}, fmt.Errorf("invalid suite topology: missing dependency alias %q", raw.Ref)
		}

		dependency, childSuite, err := r.resolveDependency(raw.Ref, entry, locks[raw.Ref])
		if err != nil {
			return resolvedTopology{}, err
		}

		child, err := r.resolveSuite(childSuite, nextStack)
		if err != nil {
			return resolvedTopology{}, err
		}
		imports[raw.ID], err = namespaceImportedTopology(raw, childSuite, child.Nodes, dependency)
		if err != nil {
			return resolvedTopology{}, err
		}
		dependencies = append(dependencies, dependency)
	}

	final := make([]TopologyNode, 0, len(rawNodes))
	for _, raw := range rawNodes {
		if raw.Kind == "suite" {
			expansion := imports[raw.ID]
			rootDependsOn := expandImportedDependencies(raw.DependsOn, imports)
			for _, node := range expansion.Nodes {
				if containsString(expansion.EntryIDs, node.ID) {
					node.DependsOn = append(append([]string{}, rootDependsOn...), node.DependsOn...)
					node.DependsOn = normalizeDependencies(node.DependsOn)
				}
				final = append(final, node)
			}
			continue
		}

		final = append(final, TopologyNode{
			ID:               raw.ID,
			Name:             raw.Name,
			Kind:             raw.Kind,
			DependsOn:        expandImportedDependencies(raw.DependsOn, imports),
			SourceSuiteID:    suite.ID,
			SourceSuiteTitle: suite.Title,
			SourceRepository: suite.Repository,
			SourceVersion:    suite.Version,
		})
	}

	for index := range final {
		final[index].Order = index
	}

	resolved, err := resolveTopology(final)
	if err != nil {
		return resolvedTopology{}, err
	}
	result := resolvedTopology{
		Nodes:        resolved,
		Dependencies: cloneResolvedDependencies(dependencies),
	}
	r.cached[id] = cloneResolvedTopology(result)
	return cloneResolvedTopology(result), nil
}

func (r *topologyResolver) resolveDependency(alias string, entry dependencyEntry, lock dependencyLockEntry) (ResolvedDependency, Definition, error) {
	ref := strings.TrimSpace(entry.Ref)
	if ref == "" {
		return ResolvedDependency{}, Definition{}, fmt.Errorf("invalid suite topology: dependency entry %q is missing ref", alias)
	}

	normalizedRef, versionFromRef := splitDependencyRef(ref)
	version := strings.TrimSpace(entry.Version)
	if version != "" && versionFromRef != "" && !dependencyHasPinnedDigest(ref) && !strings.EqualFold(version, versionFromRef) {
		return ResolvedDependency{}, Definition{}, fmt.Errorf("invalid suite topology: dependency %q declares mismatched version %q for ref %q", alias, version, ref)
	}
	if version == "" {
		if dependencyHasPinnedDigest(ref) {
			version = strings.TrimSpace(lock.Version)
		} else {
			version = versionFromRef
		}
	}
	if version == "" {
		version = strings.TrimSpace(lock.Version)
	}

	if dependencyUsesLatest(ref) || strings.EqualFold(version, "latest") {
		return ResolvedDependency{}, Definition{}, fmt.Errorf("invalid suite topology: dependency %q must use a pinned version instead of latest", alias)
	}

	lockResolved := strings.TrimSpace(lock.Resolved)
	lockDigest := firstNonEmpty(strings.TrimSpace(lock.Digest), dependencyDigest(lockResolved))
	pinnedByDigest := dependencyHasPinnedDigest(ref) || dependencyHasPinnedDigest(lockResolved) || lockDigest != ""
	if !pinnedByDigest && version == "" {
		return ResolvedDependency{}, Definition{}, fmt.Errorf("invalid suite topology: dependency %q must declare a pinned version or a locked digest", alias)
	}

	candidates := make([]Definition, 0)
	for _, suite := range r.byID {
		if suite.ID == ref || suite.ID == normalizedRef || suite.Repository == ref || suite.Repository == normalizedRef {
			candidates = append(candidates, suite)
		}
	}
	if len(candidates) == 0 {
		return ResolvedDependency{}, Definition{}, fmt.Errorf("invalid suite topology: dependency ref %q could not be resolved", ref)
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].ID < candidates[j].ID
	})

	var selected Definition
	if version == "" {
		selected = candidates[0]
	} else {
		for _, candidate := range candidates {
			if strings.TrimSpace(candidate.Version) == version {
				selected = candidate
				break
			}
		}
		if strings.TrimSpace(selected.ID) == "" {
			return ResolvedDependency{}, Definition{}, fmt.Errorf("invalid suite topology: dependency ref %q with version %q could not be resolved", normalizedRef, version)
		}
	}

	profile, err := resolveDependencyProfile(selected, firstNonEmpty(strings.TrimSpace(entry.Profile), strings.TrimSpace(lock.Profile)))
	if err != nil {
		return ResolvedDependency{}, Definition{}, err
	}

	inputs := mergeStringMaps(lock.Inputs, entry.Inputs)
	baseResolved := strings.TrimSpace(firstNonEmpty(lockResolved, ref))
	if lockDigest != "" && !strings.Contains(baseResolved, "@") {
		baseResolved = strings.TrimSpace(selected.Repository)
		if baseResolved == "" {
			baseResolved = normalizedRef
		}
		baseResolved = baseResolved + "@" + lockDigest
	}
	if baseResolved == strings.TrimSpace(ref) && version != "" && !dependencyHasPinnedDigest(baseResolved) {
		baseResolved = normalizedRef + ":" + version
	}

	dependency := ResolvedDependency{
		Alias:       alias,
		Ref:         ref,
		Version:     firstNonEmpty(version, strings.TrimSpace(selected.Version)),
		Resolved:    baseResolved,
		Digest:      firstNonEmpty(lockDigest, dependencyDigest(baseResolved), dependencyDigest(ref)),
		Profile:     profile,
		Inputs:      inputs,
		SuiteID:     selected.ID,
		SuiteTitle:  selected.Title,
		Repository:  selected.Repository,
		SourceFiles: cloneSourceFiles(selected.SourceFiles),
	}

	return dependency, selected, nil
}

func parseDependencyManifest(sourceFiles []SourceFile) (map[string]dependencyEntry, error) {
	for _, file := range sourceFiles {
		path := strings.TrimSpace(strings.ToLower(file.Path))
		if path != "dependencies.yaml" && path != "dependencies.yml" {
			continue
		}

		var document dependencyManifest
		if err := yaml.Unmarshal([]byte(file.Content), &document); err != nil {
			return nil, fmt.Errorf("invalid suite topology: could not parse %s: %w", file.Path, err)
		}

		manifest := make(map[string]dependencyEntry, len(document.Dependencies))
		for alias, entry := range document.Dependencies {
			trimmedAlias := strings.TrimSpace(alias)
			if trimmedAlias == "" {
				continue
			}
			manifest[trimmedAlias] = entry
		}
		return manifest, nil
	}

	return map[string]dependencyEntry{}, nil
}

func parseRawTopology(suiteStar string) ([]rawTopologyNode, error) {
	nodes := make([]rawTopologyNode, 0)
	for _, rawLine := range strings.Split(suiteStar, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		node, ok, err := parseRawTopologyNode(line)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		node.Order = len(nodes)
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func parseRawTopologyNode(line string) (rawTopologyNode, bool, error) {
	match := topologyAssignmentPattern.FindStringSubmatch(line)
	if len(match) == 0 {
		return rawTopologyNode{}, false, nil
	}

	call := strings.TrimSpace(match[2])
	kind, ok := topologyKind(call)
	if !ok {
		return rawTopologyNode{}, false, nil
	}

	args := match[3]
	assignment := strings.TrimSpace(match[1])
	nameMatch := topologyNamePattern.FindStringSubmatch(args)
	name := ""
	if len(nameMatch) > 0 {
		name = strings.TrimSpace(nameMatch[1])
	}

	if kind == "suite" {
		refMatch := topologyRefPattern.FindStringSubmatch(args)
		if len(refMatch) == 0 || strings.TrimSpace(refMatch[1]) == "" {
			return rawTopologyNode{}, false, fmt.Errorf("invalid suite topology: suite.run must declare ref")
		}
		if name == "" {
			name = assignment
		}
		return rawTopologyNode{
			Assignment: assignment,
			ID:         name,
			Name:       name,
			Kind:       kind,
			Ref:        strings.TrimSpace(refMatch[1]),
			DependsOn:  topologyDependencies(args),
		}, true, nil
	}

	if name == "" {
		return rawTopologyNode{}, false, nil
	}

	return rawTopologyNode{
		Assignment: assignment,
		ID:         name,
		Name:       name,
		Kind:       kind,
		DependsOn:  topologyDependencies(args),
	}, true, nil
}

func topologyKind(call string) (string, bool) {
	switch strings.TrimSpace(call) {
	case "container", "container.run", "container.create", "container.get":
		return "container", true
	case "mock", "mock.serve":
		return "mock", true
	case "service", "service.wiremock", "service.prism", "service.custom":
		return "service", true
	case "script", "script.file", "script.bash", "script.sql_migrate", "script.exec":
		return "script", true
	case "suite.run", "suite":
		return "suite", true
	case "load", "load.http", "load.grpc", "load.locust", "load.jmx", "load.k6":
		return "load", true
	case "scenario", "scenario.go", "scenario.python", "scenario.http":
		return "scenario", true
	default:
		return "", false
	}
}

func topologyDependencies(arguments string) []string {
	match := topologyAfterPattern.FindStringSubmatch(arguments)
	if len(match) == 0 || strings.TrimSpace(match[1]) == "" {
		return nil
	}

	dependsOn := make([]string, 0)
	for _, dependency := range strings.Split(match[1], ",") {
		trimmed := strings.TrimSpace(strings.ReplaceAll(dependency, "\"", ""))
		if trimmed != "" {
			dependsOn = append(dependsOn, trimmed)
		}
	}
	return normalizeDependencies(dependsOn)
}

func namespaceImportedTopology(raw rawTopologyNode, childSuite Definition, childNodes []TopologyNode, dependency ResolvedDependency) (topologyImport, error) {
	if len(childNodes) == 0 {
		return topologyImport{}, nil
	}

	profileRuntime, err := parseDependencyProfileRuntime(childSuite.SourceFiles, dependency.Profile)
	if err != nil {
		return topologyImport{}, err
	}

	rootIDs := make(map[string]struct{})
	dependants := make(map[string]int, len(childNodes))
	for _, node := range childNodes {
		if len(node.DependsOn) == 0 {
			rootIDs[node.ID] = struct{}{}
		}
		for _, dependency := range node.DependsOn {
			dependants[dependency]++
		}
	}

	namespaced := make([]TopologyNode, len(childNodes))
	idMap := make(map[string]string, len(childNodes))
	for index, node := range childNodes {
		idMap[node.ID] = raw.ID + "/" + node.ID
		namespaced[index] = node
		namespaced[index].ID = raw.ID + "/" + node.ID
		namespaced[index].Name = raw.Name + "/" + node.Name
		namespaced[index].Level = 0
		namespaced[index].Order = index
		namespaced[index].SourceSuiteID = firstNonEmpty(node.SourceSuiteID, childSuite.ID)
		namespaced[index].SourceSuiteTitle = firstNonEmpty(node.SourceSuiteTitle, childSuite.Title)
		namespaced[index].SourceRepository = firstNonEmpty(node.SourceRepository, childSuite.Repository)
		namespaced[index].SourceVersion = firstNonEmpty(node.SourceVersion, childSuite.Version)
		namespaced[index].DependencyAlias = firstNonEmpty(node.DependencyAlias, dependency.Alias)
		namespaced[index].ResolvedRef = firstNonEmpty(node.ResolvedRef, dependency.Resolved)
		namespaced[index].Digest = firstNonEmpty(node.Digest, dependency.Digest)
		namespaced[index].RuntimeProfile = firstNonEmpty(node.RuntimeProfile, dependency.Profile)
		namespaced[index].RuntimeEnv = mergeStringMaps(node.RuntimeEnv, buildDependencyRuntimeEnv(node, childSuite, dependency, profileRuntime))
		if node.Kind == "mock" {
			namespaced[index].RuntimeHeaders = mergeStringMaps(node.RuntimeHeaders, buildDependencyRuntimeHeaders(dependency))
		}
	}
	for index := range namespaced {
		remapped := make([]string, 0, len(namespaced[index].DependsOn))
		for _, dependency := range namespaced[index].DependsOn {
			if mapped := idMap[dependency]; mapped != "" {
				remapped = append(remapped, mapped)
			}
		}
		namespaced[index].DependsOn = remapped
	}

	entryIDs := make([]string, 0, len(rootIDs))
	exitIDs := make([]string, 0, len(childNodes))
	for _, node := range childNodes {
		if _, ok := rootIDs[node.ID]; ok {
			entryIDs = append(entryIDs, idMap[node.ID])
		}
		if dependants[node.ID] == 0 {
			exitIDs = append(exitIDs, idMap[node.ID])
		}
	}
	sort.Strings(entryIDs)
	sort.Strings(exitIDs)

	return topologyImport{
		EntryIDs: entryIDs,
		ExitIDs:  exitIDs,
		Nodes:    namespaced,
	}, nil
}

func expandImportedDependencies(dependsOn []string, imports map[string]topologyImport) []string {
	expanded := make([]string, 0, len(dependsOn))
	for _, dependency := range dependsOn {
		if imported, ok := imports[dependency]; ok && len(imported.ExitIDs) > 0 {
			expanded = append(expanded, imported.ExitIDs...)
			continue
		}
		expanded = append(expanded, dependency)
	}
	return normalizeDependencies(expanded)
}

func resolveTopology(nodes []TopologyNode) ([]TopologyNode, error) {
	if len(nodes) == 0 {
		return nil, nil
	}

	byID := make(map[string]*TopologyNode, len(nodes))
	for index := range nodes {
		nodes[index].DependsOn = normalizeDependencies(nodes[index].DependsOn)
		if _, exists := byID[nodes[index].ID]; exists {
			return nil, fmt.Errorf("invalid suite topology: duplicate step %q", nodes[index].ID)
		}
		byID[nodes[index].ID] = &nodes[index]
	}

	indegree := make(map[string]int, len(nodes))
	dependants := make(map[string][]string, len(nodes))
	for index := range nodes {
		node := &nodes[index]
		indegree[node.ID] = len(node.DependsOn)
		for _, dependency := range node.DependsOn {
			if _, exists := byID[dependency]; !exists {
				return nil, fmt.Errorf("invalid suite topology: %q depends on missing step %q", node.ID, dependency)
			}
			dependants[dependency] = append(dependants[dependency], node.ID)
		}
	}

	ready := make([]string, 0)
	for _, node := range nodes {
		if indegree[node.ID] == 0 {
			ready = append(ready, node.ID)
		}
	}
	sortTopologyIDs(ready, byID)

	ordered := make([]TopologyNode, 0, len(nodes))
	level := 0
	for len(ready) > 0 {
		current := append([]string{}, ready...)
		nextReady := make([]string, 0)

		for _, id := range current {
			node := *byID[id]
			node.Level = level
			ordered = append(ordered, node)

			for _, dependant := range dependants[id] {
				indegree[dependant]--
				if indegree[dependant] == 0 {
					nextReady = append(nextReady, dependant)
				}
			}
		}

		sortTopologyIDs(nextReady, byID)
		ready = nextReady
		level++
	}

	if len(ordered) != len(nodes) {
		return nil, fmt.Errorf("invalid suite topology: dependency cycle detected")
	}

	return ordered, nil
}

func sortTopologyIDs(ids []string, byID map[string]*TopologyNode) {
	sort.Slice(ids, func(i, j int) bool {
		left := byID[ids[i]]
		right := byID[ids[j]]
		if left == nil || right == nil {
			return ids[i] < ids[j]
		}
		if left.Order != right.Order {
			return left.Order < right.Order
		}
		return left.Name < right.Name
	})
}

func normalizeDependencies(dependsOn []string) []string {
	if len(dependsOn) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(dependsOn))
	result := make([]string, 0, len(dependsOn))
	for _, dependency := range dependsOn {
		trimmed := strings.TrimSpace(dependency)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func splitDependencyRef(value string) (string, string) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", ""
	}
	if repository, digest, ok := strings.Cut(trimmed, "@"); ok {
		return repository, digest
	}
	lastSlash := strings.LastIndex(trimmed, "/")
	lastColon := strings.LastIndex(trimmed, ":")
	if lastColon > lastSlash {
		return trimmed[:lastColon], trimmed[lastColon+1:]
	}
	return trimmed, ""
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
