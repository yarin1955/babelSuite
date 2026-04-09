package suites

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type rawTopologyNode struct {
	Assignment        string
	ID                string
	Name              string
	Kind              string
	Variant           string
	Arguments         string
	Ref               string
	DependsOn         []string
	ResetMocks        []string
	OnFailure         []string
	ContinueOnFailure bool
	Evaluation        *StepEvaluation
	Exports           []ArtifactExport
	Order             int
}

type parsedTopologyInvocation struct {
	Call    string
	Args    string
	Exports []ArtifactExport
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
	MockIDs  []string
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
	rawNodes = resolveAssignmentDependencies(rawNodes)
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

		node := TopologyNode{
			ID:                raw.ID,
			Name:              raw.Name,
			Kind:              raw.Kind,
			Variant:           raw.Variant,
			DependsOn:         expandImportedDependencies(append(append([]string{}, raw.DependsOn...), raw.OnFailure...), imports),
			ResetMocks:        expandImportedMockTargets(raw.ResetMocks, imports),
			OnFailure:         expandImportedDependencies(raw.OnFailure, imports),
			ContinueOnFailure: raw.ContinueOnFailure,
			Evaluation:        cloneStepEvaluation(raw.Evaluation),
			ArtifactExports:   append([]ArtifactExport{}, raw.Exports...),
			SourceSuiteID:     suite.ID,
			SourceSuiteTitle:  suite.Title,
			SourceRepository:  suite.Repository,
			SourceVersion:     suite.Version,
		}
		if raw.Kind == "traffic" {
			spec, err := resolveLoadSpec(raw, suite.SourceFiles)
			if err != nil {
				return resolvedTopology{}, err
			}
			node.Load = spec
		}
		final = append(final, node)
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
	if nodes, err := evalStarlarkTopology(suiteStar); err == nil {
		return nodes, nil
	}
	nodes := make([]rawTopologyNode, 0)
	for _, line := range topologyStatements(suiteStar) {
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
	assignment, expression, ok := parseTopologyAssignment(line)
	if !ok {
		return rawTopologyNode{}, false, nil
	}

	invocation, ok, err := parseTopologyInvocation(expression)
	if err != nil {
		return rawTopologyNode{}, false, err
	}
	if !ok {
		return rawTopologyNode{}, false, nil
	}

	call := canonicalRuntimeCall(invocation.Call)
	switch call {
	case "container":
		return rawTopologyNode{}, false, fmt.Errorf("invalid suite topology: use service.run instead of container")
	case "mock":
		return rawTopologyNode{}, false, fmt.Errorf("invalid suite topology: use service.mock instead of mock")
	case "service":
		return rawTopologyNode{}, false, fmt.Errorf("invalid suite topology: use service.run, service.mock, service.wiremock, service.prism, or service.custom instead of service")
	case "script":
		return rawTopologyNode{}, false, fmt.Errorf("invalid suite topology: use task.run instead of script")
	case "task":
		return rawTopologyNode{}, false, fmt.Errorf("invalid suite topology: use task.run instead of task")
	case "load":
		return rawTopologyNode{}, false, fmt.Errorf("invalid suite topology: use an explicit traffic profile like traffic.smoke, traffic.baseline, traffic.stress, traffic.spike, traffic.soak, traffic.scalability, traffic.step, traffic.wave, traffic.staged, traffic.constant_throughput, traffic.constant_pacing, or traffic.open_model instead of load")
	case "traffic":
		return rawTopologyNode{}, false, fmt.Errorf("invalid suite topology: use an explicit traffic profile like traffic.smoke, traffic.baseline, traffic.stress, traffic.spike, traffic.soak, traffic.scalability, traffic.step, traffic.wave, traffic.staged, traffic.constant_throughput, traffic.constant_pacing, or traffic.open_model instead of traffic")
	case "scenario":
		return rawTopologyNode{}, false, fmt.Errorf("invalid suite topology: use test.run instead of scenario")
	case "test":
		return rawTopologyNode{}, false, fmt.Errorf("invalid suite topology: use test.run instead of test")
	case "suite":
		return rawTopologyNode{}, false, fmt.Errorf("invalid suite topology: use suite.run instead of suite")
	}
	kind, ok := topologyKind(call)
	if !ok {
		if runtimeNamespace(call) != "" {
			return rawTopologyNode{}, false, fmt.Errorf("invalid suite topology: unsupported runtime call %q", call)
		}
		return rawTopologyNode{}, false, nil
	}

	args := invocation.Args
	name := firstNonEmpty(topologyNamedStringArgument(args, "name", "name_or_id", "id"), assignment)
	evaluation, err := parseStepEvaluation(args)
	if err != nil {
		return rawTopologyNode{}, false, err
	}
	onFailure := topologyRunConditions(args, "on_failure")
	resetMocks := topologyRunConditions(args, "reset_mocks")
	continueOnFailure, err := parseTopologyBooleanArgument(args, "continue_on_failure")
	if err != nil {
		return rawTopologyNode{}, false, fmt.Errorf("invalid suite topology: continue_on_failure must be true or false: %w", err)
	}

	if kind == "suite" {
		ref := topologyNamedStringArgument(args, "ref")
		if ref == "" {
			return rawTopologyNode{}, false, fmt.Errorf("invalid suite topology: suite.run must declare ref")
		}
		if evaluation != nil {
			return rawTopologyNode{}, false, fmt.Errorf("invalid suite topology: suite.run does not support evaluation controls")
		}
		if len(onFailure) > 0 {
			return rawTopologyNode{}, false, fmt.Errorf("invalid suite topology: suite.run does not support on_failure hooks")
		}
		if len(resetMocks) > 0 {
			return rawTopologyNode{}, false, fmt.Errorf("invalid suite topology: suite.run does not support reset_mocks")
		}
		if continueOnFailure {
			return rawTopologyNode{}, false, fmt.Errorf("invalid suite topology: suite.run does not support continue_on_failure")
		}
		if len(invocation.Exports) > 0 {
			return rawTopologyNode{}, false, fmt.Errorf("invalid suite topology: suite.run does not support artifact exports")
		}
		if name == "" {
			name = assignment
		}
		return rawTopologyNode{
			Assignment: assignment,
			ID:         name,
			Name:       name,
			Kind:       kind,
			Variant:    call,
			Arguments:  args,
			Ref:        ref,
			DependsOn:  topologyDependencies(args),
			ResetMocks: resetMocks,
			OnFailure:  onFailure,
			Evaluation: evaluation,
			Exports:    append([]ArtifactExport{}, invocation.Exports...),
		}, true, nil
	}

	return rawTopologyNode{
		Assignment:        assignment,
		ID:                name,
		Name:              name,
		Kind:              kind,
		Variant:           call,
		Arguments:         args,
		DependsOn:         topologyDependencies(args),
		ResetMocks:        resetMocks,
		OnFailure:         onFailure,
		ContinueOnFailure: continueOnFailure,
		Evaluation:        evaluation,
		Exports:           append([]ArtifactExport{}, invocation.Exports...),
	}, true, nil
}

func runtimeNamespace(call string) string {
	trimmed := strings.TrimSpace(call)
	if trimmed == "" {
		return ""
	}
	if index := strings.Index(trimmed, "."); index >= 0 {
		trimmed = trimmed[:index]
	}
	switch trimmed {
	case "container", "mock", "service", "script", "task", "load", "traffic", "scenario", "test", "suite":
		return trimmed
	default:
		return ""
	}
}

func parseTopologyAssignment(line string) (string, string, bool) {
	separator := findTopologyAssignmentSeparator(line)
	if separator <= 0 {
		return "", "", false
	}

	left := strings.TrimSpace(line[:separator])
	right := strings.TrimSpace(line[separator+1:])
	if !isTopologyIdentifier(left) || right == "" {
		return "", "", false
	}
	return left, right, true
}

func findTopologyAssignmentSeparator(line string) int {
	depthParen := 0
	depthBracket := 0
	depthBrace := 0
	inString := false
	escaped := false

	for index, ch := range line {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}

		switch ch {
		case '"':
			inString = true
		case '(':
			depthParen++
		case ')':
			if depthParen > 0 {
				depthParen--
			}
		case '[':
			depthBracket++
		case ']':
			if depthBracket > 0 {
				depthBracket--
			}
		case '{':
			depthBrace++
		case '}':
			if depthBrace > 0 {
				depthBrace--
			}
		case '=':
			if depthParen == 0 && depthBracket == 0 && depthBrace == 0 {
				return index
			}
		}
	}

	return -1
}

func topologyStatements(suiteStar string) []string {
	return scanLogicalStatements(suiteStar)
}

func parseTopologyInvocation(expression string) (parsedTopologyInvocation, bool, error) {
	openIndex := strings.Index(expression, "(")
	if openIndex <= 0 {
		return parsedTopologyInvocation{}, false, nil
	}

	call := strings.TrimSpace(expression[:openIndex])
	if !isTopologyCallPath(call) {
		return parsedTopologyInvocation{}, false, nil
	}

	args, nextIndex, err := scanCallArguments(expression, openIndex)
	if err != nil {
		return parsedTopologyInvocation{}, false, fmt.Errorf("invalid suite topology: malformed call for %s: %w", call, err)
	}

	invocation := parsedTopologyInvocation{
		Call: call,
		Args: args,
	}

	rest := strings.TrimSpace(expression[nextIndex:])
	for rest != "" {
		if !strings.HasPrefix(rest, ".") {
			return parsedTopologyInvocation{}, false, fmt.Errorf("invalid suite topology: unsupported chained expression %q", rest)
		}

		rest = strings.TrimSpace(rest[1:])
		exportIndex := strings.Index(rest, "(")
		if exportIndex <= 0 {
			return parsedTopologyInvocation{}, false, fmt.Errorf("invalid suite topology: malformed chained call %q", rest)
		}

		method := strings.TrimSpace(rest[:exportIndex])
		arguments, consumed, err := scanCallArguments(rest, exportIndex)
		if err != nil {
			return parsedTopologyInvocation{}, false, fmt.Errorf("invalid suite topology: malformed chained call %q: %w", method, err)
		}

		if method != "export" {
			return parsedTopologyInvocation{}, false, fmt.Errorf("invalid suite topology: unsupported chained call %q", method)
		}

		export, err := parseArtifactExport(arguments)
		if err != nil {
			return parsedTopologyInvocation{}, false, err
		}
		invocation.Exports = append(invocation.Exports, export)
		rest = strings.TrimSpace(rest[consumed:])
	}

	return invocation, true, nil
}

func isTopologyCallPath(value string) bool {
	if value == "" {
		return false
	}
	for _, part := range strings.Split(value, ".") {
		if !isTopologyIdentifier(part) {
			return false
		}
	}
	return true
}

func scanCallArguments(expression string, openIndex int) (string, int, error) {
	if openIndex < 0 || openIndex >= len(expression) || expression[openIndex] != '(' {
		return "", 0, fmt.Errorf("missing opening parenthesis")
	}

	depth := 1
	inString := false
	escaped := false
	for index := openIndex + 1; index < len(expression); index++ {
		ch := expression[index]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}

		switch ch {
		case '"':
			inString = true
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return expression[openIndex+1 : index], index + 1, nil
			}
		}
	}

	return "", 0, fmt.Errorf("missing closing parenthesis")
}

func parseArtifactExport(arguments string) (ArtifactExport, error) {
	export := ArtifactExport{On: "success"}
	parts := splitTopLevel(arguments)
	if len(parts) == 0 {
		return ArtifactExport{}, fmt.Errorf("invalid suite topology: export() requires a path")
	}

	for index, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if strings.Contains(part, "=") {
			key, value, ok := strings.Cut(part, "=")
			if !ok {
				return ArtifactExport{}, fmt.Errorf("invalid suite topology: malformed export argument %q", part)
			}
			key = strings.TrimSpace(key)
			value = strings.TrimSpace(value)
			switch key {
			case "path":
				resolved, err := unquoteTopologyString(value)
				if err != nil {
					return ArtifactExport{}, fmt.Errorf("invalid suite topology: invalid export path: %w", err)
				}
				export.Path = resolved
			case "name":
				resolved, err := unquoteTopologyString(value)
				if err != nil {
					return ArtifactExport{}, fmt.Errorf("invalid suite topology: invalid export name: %w", err)
				}
				export.Name = resolved
			case "on":
				resolved, err := unquoteTopologyString(value)
				if err != nil {
					return ArtifactExport{}, fmt.Errorf("invalid suite topology: invalid export trigger: %w", err)
				}
				export.On = strings.TrimSpace(resolved)
			case "format":
				resolved, err := unquoteTopologyString(value)
				if err != nil {
					return ArtifactExport{}, fmt.Errorf("invalid suite topology: invalid export format: %w", err)
				}
				export.Format = strings.ToLower(strings.TrimSpace(resolved))
			default:
				return ArtifactExport{}, fmt.Errorf("invalid suite topology: unsupported export argument %q", key)
			}
			continue
		}

		if index > 0 || export.Path != "" {
			return ArtifactExport{}, fmt.Errorf("invalid suite topology: export() only accepts one positional path")
		}

		resolved, err := unquoteTopologyString(part)
		if err != nil {
			return ArtifactExport{}, fmt.Errorf("invalid suite topology: invalid export path: %w", err)
		}
		export.Path = resolved
	}

	if strings.TrimSpace(export.Path) == "" {
		return ArtifactExport{}, fmt.Errorf("invalid suite topology: export() requires a path")
	}

	switch export.On {
	case "", "success":
		export.On = "success"
	case "failure", "always":
	default:
		return ArtifactExport{}, fmt.Errorf("invalid suite topology: unsupported export trigger %q", export.On)
	}

	return export, nil
}

func splitTopLevel(arguments string) []string {
	parts := make([]string, 0)
	var current strings.Builder
	depthParen := 0
	depthBracket := 0
	depthBrace := 0
	inString := false
	escaped := false

	flush := func() {
		part := strings.TrimSpace(current.String())
		if part != "" {
			parts = append(parts, part)
		}
		current.Reset()
	}

	for _, ch := range arguments {
		if inString {
			current.WriteRune(ch)
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}

		switch ch {
		case '"':
			inString = true
			current.WriteRune(ch)
		case '(':
			depthParen++
			current.WriteRune(ch)
		case ')':
			if depthParen > 0 {
				depthParen--
			}
			current.WriteRune(ch)
		case '[':
			depthBracket++
			current.WriteRune(ch)
		case ']':
			if depthBracket > 0 {
				depthBracket--
			}
			current.WriteRune(ch)
		case '{':
			depthBrace++
			current.WriteRune(ch)
		case '}':
			if depthBrace > 0 {
				depthBrace--
			}
			current.WriteRune(ch)
		case ',':
			if depthParen == 0 && depthBracket == 0 && depthBrace == 0 {
				flush()
				continue
			}
			current.WriteRune(ch)
		default:
			current.WriteRune(ch)
		}
	}

	flush()
	return parts
}

func unquoteTopologyString(value string) (string, error) {
	resolved, err := strconv.Unquote(strings.TrimSpace(value))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resolved), nil
}

func topologyKind(call string) (string, bool) {
	switch strings.TrimSpace(call) {
	case "service.mock":
		return "mock", true
	case "service.run", "service.wiremock", "service.prism", "service.custom":
		return "service", true
	case "task.run":
		return "task", true
	case "test.run":
		return "test", true
	case "suite.run":
		return "suite", true
	case "traffic.smoke", "traffic.baseline", "traffic.stress", "traffic.spike", "traffic.soak", "traffic.scalability", "traffic.step", "traffic.wave", "traffic.staged", "traffic.constant_throughput", "traffic.constant_pacing", "traffic.open_model":
		return "traffic", true
	default:
		return "", false
	}
}

func canonicalRuntimeCall(call string) string {
	trimmed := strings.TrimSpace(call)
	if strings.HasPrefix(trimmed, "load.") {
		return "traffic." + strings.TrimPrefix(trimmed, "load.")
	}
	switch trimmed {
	case "mock.serve":
		return "service.mock"
	default:
		return trimmed
	}
}

func canonicalTrafficCall(call string) string {
	trimmed := canonicalRuntimeCall(call)
	if strings.HasPrefix(trimmed, "traffic.") {
		return trimmed
	}
	return trimmed
}

func topologyDependencies(arguments string) []string {
	value := topologyNamedArgument(arguments, "after")
	if value == "" {
		return nil
	}

	return normalizeDependencies(parseTopologyDependencyList(value))
}

func topologyRunConditions(arguments string, keys ...string) []string {
	result := make([]string, 0)
	for _, key := range keys {
		result = append(result, parseTopologyDependencyList(topologyNamedArgument(arguments, key))...)
	}
	return normalizeDependencies(result)
}

func topologyNamedStringArgument(arguments string, keys ...string) string {
	value := topologyNamedArgument(arguments, keys...)
	if value == "" {
		return ""
	}
	resolved, err := unquoteTopologyString(value)
	if err != nil {
		return ""
	}
	return resolved
}

func parseTopologyBooleanArgument(arguments string, keys ...string) (bool, error) {
	value := topologyNamedArgument(arguments, keys...)
	if value == "" {
		return false, nil
	}
	return parseTopologyBoolean(value)
}

func topologyNamedArgument(arguments string, keys ...string) string {
	if len(keys) == 0 {
		return ""
	}

	keySet := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		keySet[strings.TrimSpace(key)] = struct{}{}
	}

	for _, part := range splitTopLevel(arguments) {
		key, value, ok := splitNamedArgument(part)
		if !ok {
			continue
		}
		if _, exists := keySet[strings.TrimSpace(key)]; !exists {
			continue
		}
		return strings.TrimSpace(value)
	}

	return ""
}

func parseStepEvaluation(arguments string) (*StepEvaluation, error) {
	evaluation := &StepEvaluation{}
	hasEvaluation := false

	if value := topologyNamedArgument(arguments, "expect_exit"); value != "" {
		parsed, err := parseTopologyInteger(value)
		if err != nil {
			return nil, fmt.Errorf("invalid suite topology: expect_exit must be an integer: %w", err)
		}
		evaluation.ExpectExit = &parsed
		hasEvaluation = true
	}
	if value := topologyNamedArgument(arguments, "expect_logs"); value != "" {
		parsed, err := parseTopologyStringMatchers(value)
		if err != nil {
			return nil, fmt.Errorf("invalid suite topology: expect_logs must be a string or list of strings: %w", err)
		}
		evaluation.ExpectLogs = parsed
		hasEvaluation = hasEvaluation || len(parsed) > 0
	}
	if value := topologyNamedArgument(arguments, "fail_on_logs"); value != "" {
		parsed, err := parseTopologyStringMatchers(value)
		if err != nil {
			return nil, fmt.Errorf("invalid suite topology: fail_on_logs must be a string or list of strings: %w", err)
		}
		evaluation.FailOnLogs = parsed
		hasEvaluation = hasEvaluation || len(parsed) > 0
	}

	if !hasEvaluation {
		return nil, nil
	}
	return evaluation, nil
}

func parseTopologyInteger(value string) (int, error) {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, "\"") {
		resolved, err := unquoteTopologyString(trimmed)
		if err != nil {
			return 0, err
		}
		trimmed = resolved
	}
	return strconv.Atoi(strings.TrimSpace(trimmed))
}

func parseTopologyBoolean(value string) (bool, error) {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, "\"") {
		resolved, err := unquoteTopologyString(trimmed)
		if err != nil {
			return false, err
		}
		trimmed = resolved
	}
	switch strings.ToLower(strings.TrimSpace(trimmed)) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean %q", value)
	}
}

func parseTopologyStringMatchers(value string) ([]string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}
	if strings.HasPrefix(trimmed, "[") {
		items := parseTopologyDependencyList(trimmed)
		result := make([]string, 0, len(items))
		for _, item := range items {
			if strings.TrimSpace(item) == "" {
				continue
			}
			result = append(result, item)
		}
		return result, nil
	}
	resolved, err := unquoteTopologyString(trimmed)
	if err != nil {
		return nil, err
	}
	if resolved == "" {
		return nil, nil
	}
	return []string{resolved}, nil
}

func splitNamedArgument(part string) (string, string, bool) {
	depthParen := 0
	depthBracket := 0
	depthBrace := 0
	inString := false
	escaped := false

	for index, ch := range part {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}

		switch ch {
		case '"':
			inString = true
		case '(':
			depthParen++
		case ')':
			if depthParen > 0 {
				depthParen--
			}
		case '[':
			depthBracket++
		case ']':
			if depthBracket > 0 {
				depthBracket--
			}
		case '{':
			depthBrace++
		case '}':
			if depthBrace > 0 {
				depthBrace--
			}
		case '=':
			if depthParen != 0 || depthBracket != 0 || depthBrace != 0 {
				continue
			}
			key := strings.TrimSpace(part[:index])
			if !isTopologyIdentifier(key) {
				return "", "", false
			}
			return key, strings.TrimSpace(part[index+1:]), true
		}
	}

	return "", "", false
}

func parseTopologyDependencyList(value string) []string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) < 2 || trimmed[0] != '[' || trimmed[len(trimmed)-1] != ']' {
		return nil
	}

	items := splitTopLevel(trimmed[1 : len(trimmed)-1])
	dependsOn := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if strings.HasPrefix(item, "\"") {
			resolved, err := unquoteTopologyString(item)
			if err != nil || resolved == "" {
				continue
			}
			dependsOn = append(dependsOn, resolved)
			continue
		}
		dependsOn = append(dependsOn, item)
	}
	return dependsOn
}

func isTopologyIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for index, ch := range value {
		switch {
		case index == 0 && (ch == '_' || isASCIIAlpha(ch)):
		case index > 0 && (ch == '_' || isASCIIAlpha(ch) || isASCIIDigit(ch)):
		default:
			return false
		}
	}
	return true
}

func isASCIIAlpha(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func isASCIIDigit(ch rune) bool {
	return ch >= '0' && ch <= '9'
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
		namespaced[index].Load = cloneLoadSpec(node.Load)
		namespaced[index].Evaluation = cloneStepEvaluation(node.Evaluation)
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
		if len(namespaced[index].OnFailure) > 0 {
			remappedOnFailure := make([]string, 0, len(namespaced[index].OnFailure))
			for _, dependency := range namespaced[index].OnFailure {
				if mapped := idMap[dependency]; mapped != "" {
					remappedOnFailure = append(remappedOnFailure, mapped)
				}
			}
			namespaced[index].OnFailure = remappedOnFailure
		}
		if len(namespaced[index].ResetMocks) > 0 {
			remappedResetMocks := make([]string, 0, len(namespaced[index].ResetMocks))
			for _, dependency := range namespaced[index].ResetMocks {
				if mapped := idMap[dependency]; mapped != "" {
					remappedResetMocks = append(remappedResetMocks, mapped)
				}
			}
			namespaced[index].ResetMocks = normalizeDependencies(remappedResetMocks)
		}
	}

	entryIDs := make([]string, 0, len(rootIDs))
	exitIDs := make([]string, 0, len(childNodes))
	mockIDs := make([]string, 0, len(childNodes))
	for _, node := range childNodes {
		if _, ok := rootIDs[node.ID]; ok {
			entryIDs = append(entryIDs, idMap[node.ID])
		}
		if dependants[node.ID] == 0 {
			exitIDs = append(exitIDs, idMap[node.ID])
		}
		if node.Kind == "mock" {
			mockIDs = append(mockIDs, idMap[node.ID])
		}
	}
	sort.Strings(entryIDs)
	sort.Strings(exitIDs)
	sort.Strings(mockIDs)

	return topologyImport{
		EntryIDs: entryIDs,
		ExitIDs:  exitIDs,
		MockIDs:  mockIDs,
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

func expandImportedMockTargets(targets []string, imports map[string]topologyImport) []string {
	expanded := make([]string, 0, len(targets))
	for _, target := range targets {
		if imported, ok := imports[target]; ok && len(imported.MockIDs) > 0 {
			expanded = append(expanded, imported.MockIDs...)
			continue
		}
		expanded = append(expanded, target)
	}
	return normalizeDependencies(expanded)
}

func resolveAssignmentDependencies(nodes []rawTopologyNode) []rawTopologyNode {
	if len(nodes) == 0 {
		return nil
	}

	assignmentToID := make(map[string]string, len(nodes))
	for _, node := range nodes {
		if strings.TrimSpace(node.Assignment) == "" || strings.TrimSpace(node.ID) == "" {
			continue
		}
		assignmentToID[node.Assignment] = node.ID
	}

	resolved := make([]rawTopologyNode, len(nodes))
	copy(resolved, nodes)
	for index := range resolved {
		if len(resolved[index].DependsOn) > 0 {
			remapped := make([]string, 0, len(resolved[index].DependsOn))
			for _, dependency := range resolved[index].DependsOn {
				if mapped, ok := assignmentToID[dependency]; ok {
					remapped = append(remapped, mapped)
					continue
				}
				remapped = append(remapped, dependency)
			}
			resolved[index].DependsOn = normalizeDependencies(remapped)
		}
		if len(resolved[index].OnFailure) > 0 {
			remapped := make([]string, 0, len(resolved[index].OnFailure))
			for _, dependency := range resolved[index].OnFailure {
				if mapped, ok := assignmentToID[dependency]; ok {
					remapped = append(remapped, mapped)
					continue
				}
				remapped = append(remapped, dependency)
			}
			resolved[index].OnFailure = normalizeDependencies(remapped)
		}
		if len(resolved[index].ResetMocks) > 0 {
			remapped := make([]string, 0, len(resolved[index].ResetMocks))
			for _, dependency := range resolved[index].ResetMocks {
				if mapped, ok := assignmentToID[dependency]; ok {
					remapped = append(remapped, mapped)
					continue
				}
				remapped = append(remapped, dependency)
			}
			resolved[index].ResetMocks = normalizeDependencies(remapped)
		}
	}
	return resolved
}

func resolveTopology(nodes []TopologyNode) ([]TopologyNode, error) {
	if len(nodes) == 0 {
		return nil, nil
	}

	byID := make(map[string]*TopologyNode, len(nodes))
	for index := range nodes {
		nodes[index].DependsOn = normalizeDependencies(nodes[index].DependsOn)
		nodes[index].ResetMocks = normalizeDependencies(nodes[index].ResetMocks)
		if _, exists := byID[nodes[index].ID]; exists {
			return nil, fmt.Errorf("invalid suite topology: duplicate step %q", nodes[index].ID)
		}
		byID[nodes[index].ID] = &nodes[index]
	}

	for index := range nodes {
		if len(nodes[index].ResetMocks) == 0 {
			continue
		}
		if nodes[index].Kind != "test" {
			return nil, fmt.Errorf("invalid suite topology: reset_mocks is only supported on test.run nodes")
		}
		for _, target := range nodes[index].ResetMocks {
			dependency := byID[target]
			if dependency == nil {
				return nil, fmt.Errorf("invalid suite topology: %q resets missing mock %q", nodes[index].ID, target)
			}
			if dependency.Kind != "mock" {
				return nil, fmt.Errorf("invalid suite topology: %q reset_mocks target %q is not a mock node", nodes[index].ID, target)
			}
		}
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

func cloneStepEvaluation(input *StepEvaluation) *StepEvaluation {
	if input == nil {
		return nil
	}

	output := *input
	output.ExpectLogs = append([]string{}, input.ExpectLogs...)
	output.FailOnLogs = append([]string{}, input.FailOnLogs...)
	if input.ExpectExit != nil {
		value := *input.ExpectExit
		output.ExpectExit = &value
	}
	return &output
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
