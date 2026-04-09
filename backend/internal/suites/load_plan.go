package suites

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	maxLoadPlanBytes   = 128 * 1024
	maxLoadUsers       = 32
	maxLoadStages      = 12
	maxLoadTasks       = 24
	maxLoadDuration    = 5 * time.Minute
	maxLoadRequestRate = 100.0
)

type loadPlanContext struct {
	users      map[string]LoadUser
	tasks      map[string]LoadTask
	requests   map[string]LoadRequest
	waits      map[string]LoadWait
	thresholds map[string]LoadThreshold
	stages     map[string]LoadStage
}

func resolveLoadSpec(raw rawTopologyNode, sourceFiles []SourceFile) (*LoadSpec, error) {
	planPath := normalizeLoadPlanPath(topologyNamedStringArgument(raw.Arguments, "plan"))
	if planPath == "" {
		return nil, fmt.Errorf("invalid suite topology: %s must declare plan", raw.Variant)
	}

	target := strings.TrimSpace(topologyNamedStringArgument(raw.Arguments, "target"))
	if target == "" {
		return nil, fmt.Errorf("invalid suite topology: %s must declare target", raw.Variant)
	}

	content, ok := sourceFileContent(sourceFiles, planPath)
	if !ok {
		if len(sourceFiles) == 0 {
			return &LoadSpec{
				Variant:      raw.Variant,
				PlanPath:     planPath,
				Target:       target,
				RequestsPerS: topologyNamedNumberArgument(raw.Arguments, "rps", "target_rps"),
				ArrivalRate:  topologyNamedNumberArgument(raw.Arguments, "arrival_rate"),
			}, nil
		}
		return nil, fmt.Errorf("invalid suite topology: traffic plan %q could not be resolved", planPath)
	}
	if len(content) > maxLoadPlanBytes {
		return nil, fmt.Errorf("invalid suite topology: traffic plan %q exceeds the %d byte safety limit", planPath, maxLoadPlanBytes)
	}

	spec := &LoadSpec{
		Variant:      raw.Variant,
		PlanPath:     planPath,
		Target:       target,
		RequestsPerS: topologyNamedNumberArgument(raw.Arguments, "rps", "target_rps"),
		ArrivalRate:  topologyNamedNumberArgument(raw.Arguments, "arrival_rate"),
	}
	return parseLoadPlan(spec, content)
}

func sourceFileContent(sourceFiles []SourceFile, path string) (string, bool) {
	normalized := strings.Trim(strings.TrimSpace(path), "/")
	for _, file := range sourceFiles {
		if strings.Trim(strings.TrimSpace(file.Path), "/") != normalized {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(file.Content), "# Missing example source") {
			return "", false
		}
		return file.Content, true
	}
	return "", false
}

func normalizeLoadPlanPath(value string) string {
	trimmed := strings.Trim(strings.TrimSpace(value), "/")
	trimmed = strings.TrimPrefix(trimmed, "./")
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "traffic/") {
		return trimmed
	}
	if strings.HasPrefix(trimmed, "load/") {
		return normalizeSourcePath("traffic", strings.TrimPrefix(trimmed, "load/"))
	}
	return normalizeSourcePath("traffic", trimmed)
}

func parseLoadPlan(base *LoadSpec, source string) (*LoadSpec, error) {
	contextState := loadPlanContext{
		users:      make(map[string]LoadUser),
		tasks:      make(map[string]LoadTask),
		requests:   make(map[string]LoadRequest),
		waits:      make(map[string]LoadWait),
		thresholds: make(map[string]LoadThreshold),
		stages:     make(map[string]LoadStage),
	}

	spec := cloneLoadSpec(base)
	if spec == nil {
		spec = &LoadSpec{}
	}

	foundPlan := false
	for _, statement := range scanLogicalStatements(source) {
		assignment, expression, assigned := parseTopologyAssignment(statement)
		expr := strings.TrimSpace(statement)
		if assigned {
			expr = expression
		}

		invocation, ok, err := parseTopologyInvocation(expr)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}

		switch canonicalTrafficCall(invocation.Call) {
		case "traffic.user":
			user, err := parseLoadUser(assignment, invocation.Args, contextState)
			if err != nil {
				return nil, err
			}
			contextState.users[assignment] = user
		case "traffic.task":
			task, err := parseLoadTask(assignment, invocation.Args, contextState)
			if err != nil {
				return nil, err
			}
			contextState.tasks[assignment] = task
		case "traffic.get", "traffic.post":
			request, err := parseLoadRequest(invocation.Call, invocation.Args)
			if err != nil {
				return nil, err
			}
			contextState.requests[assignment] = request
		case "traffic.constant", "traffic.between", "traffic.pacing":
			wait, err := parseLoadWait(invocation.Call, invocation.Args)
			if err != nil {
				return nil, err
			}
			contextState.waits[assignment] = wait
		case "traffic.threshold":
			threshold, err := parseLoadThreshold(invocation.Args)
			if err != nil {
				return nil, err
			}
			contextState.thresholds[assignment] = threshold
		case "traffic.stage":
			stage, err := parseLoadStage(invocation.Args)
			if err != nil {
				return nil, err
			}
			contextState.stages[assignment] = stage
		case "traffic.plan":
			if foundPlan {
				return nil, fmt.Errorf("invalid suite topology: traffic plan %q declares traffic.plan more than once", spec.PlanPath)
			}
			if err := applyLoadPlan(spec, invocation.Args, contextState); err != nil {
				return nil, err
			}
			foundPlan = true
		}
	}

	if !foundPlan {
		return nil, fmt.Errorf("invalid suite topology: traffic plan %q must declare traffic.plan(...)", spec.PlanPath)
	}
	if err := validateLoadSpec(spec); err != nil {
		return nil, err
	}
	return spec, nil
}

func applyLoadPlan(spec *LoadSpec, arguments string, contextState loadPlanContext) error {
	usersValue := topologyNamedArgument(arguments, "users")
	users, err := parseLoadUserList(usersValue, contextState)
	if err != nil {
		return err
	}
	spec.Users = users

	shapeValue := topologyNamedArgument(arguments, "shape")
	stages, err := parseLoadStages(shapeValue, contextState)
	if err != nil {
		return err
	}
	spec.Stages = stages

	if thresholdsValue := topologyNamedArgument(arguments, "thresholds"); thresholdsValue != "" {
		thresholds, err := parseLoadThresholdList(thresholdsValue, contextState)
		if err != nil {
			return err
		}
		spec.Thresholds = thresholds
	}

	return nil
}

func parseLoadUser(assignment string, arguments string, contextState loadPlanContext) (LoadUser, error) {
	user := LoadUser{
		ID:     assignment,
		Name:   firstNonEmpty(topologyNamedStringArgument(arguments, "name", "id"), assignment),
		Weight: int(topologyNamedNumberArgument(arguments, "weight")),
	}
	if user.Weight <= 0 {
		user.Weight = 1
	}

	wait, err := parseLoadWaitExpression(topologyNamedArgument(arguments, "wait"), contextState)
	if err != nil {
		return LoadUser{}, err
	}
	user.Wait = wait

	tasks, err := parseLoadTaskList(topologyNamedArgument(arguments, "tasks"), contextState)
	if err != nil {
		return LoadUser{}, err
	}
	user.Tasks = tasks
	return user, nil
}

func parseLoadTask(assignment string, arguments string, contextState loadPlanContext) (LoadTask, error) {
	request, err := parseLoadRequestExpression(topologyNamedArgument(arguments, "request"), contextState)
	if err != nil {
		return LoadTask{}, err
	}

	task := LoadTask{
		ID:      assignment,
		Name:    firstNonEmpty(topologyNamedStringArgument(arguments, "name", "id"), request.Name, assignment),
		Weight:  int(topologyNamedNumberArgument(arguments, "weight")),
		Request: request,
	}
	if task.Weight <= 0 {
		task.Weight = 1
	}

	if checksValue := topologyNamedArgument(arguments, "checks"); checksValue != "" {
		checks, err := parseLoadThresholdList(checksValue, contextState)
		if err != nil {
			return LoadTask{}, err
		}
		task.Checks = append(task.Checks, checks...)
	}
	task.Checks = append(task.Checks, task.Request.Checks...)
	return task, nil
}

func parseLoadRequestExpression(value string, contextState loadPlanContext) (LoadRequest, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return LoadRequest{}, fmt.Errorf("invalid suite topology: traffic.task must declare request")
	}
	if request, ok := contextState.requests[trimmed]; ok {
		return request, nil
	}

	invocation, ok, err := parseTopologyInvocation(trimmed)
	if err != nil {
		return LoadRequest{}, err
	}
	if !ok {
		return LoadRequest{}, fmt.Errorf("invalid suite topology: unsupported traffic request %q", trimmed)
	}
	return parseLoadRequest(invocation.Call, invocation.Args)
}

func parseLoadRequest(call string, arguments string) (LoadRequest, error) {
	call = canonicalTrafficCall(call)
	request := LoadRequest{
		Method: strings.ToUpper(strings.TrimPrefix(call, "traffic.")),
		Path:   firstNonEmpty(parsePositionalStringArgument(arguments, 0), topologyNamedStringArgument(arguments, "path")),
		Name:   topologyNamedStringArgument(arguments, "name"),
	}
	if request.Path == "" {
		return LoadRequest{}, fmt.Errorf("invalid suite topology: %s requires a request path", call)
	}
	if request.Name == "" {
		request.Name = request.Path
	}

	if headersValue := topologyNamedArgument(arguments, "headers"); headersValue != "" {
		headers, err := parseStringMapLiteral(headersValue)
		if err != nil {
			return LoadRequest{}, err
		}
		request.Headers = headers
	}
	if bodyValue := topologyNamedArgument(arguments, "json"); bodyValue != "" {
		body, err := parseJSONLiteral(bodyValue)
		if err != nil {
			return LoadRequest{}, err
		}
		request.Body = body
		if request.Headers == nil {
			request.Headers = map[string]string{}
		}
		if request.Headers["Content-Type"] == "" {
			request.Headers["Content-Type"] = "application/json"
		}
	}
	if checksValue := topologyNamedArgument(arguments, "checks"); checksValue != "" {
		checks, err := parseLoadThresholdListInline(checksValue)
		if err != nil {
			return LoadRequest{}, err
		}
		request.Checks = checks
	}
	return request, nil
}

func parseLoadWaitExpression(value string, contextState loadPlanContext) (LoadWait, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return LoadWait{}, nil
	}
	if wait, ok := contextState.waits[trimmed]; ok {
		return wait, nil
	}

	invocation, ok, err := parseTopologyInvocation(trimmed)
	if err != nil {
		return LoadWait{}, err
	}
	if !ok {
		return LoadWait{}, fmt.Errorf("invalid suite topology: unsupported traffic wait expression %q", trimmed)
	}
	return parseLoadWait(invocation.Call, invocation.Args)
}

func parseLoadWait(call string, arguments string) (LoadWait, error) {
	switch canonicalTrafficCall(call) {
	case "traffic.constant":
		seconds, err := parsePositionalFloatArgument(arguments, 0)
		if err != nil {
			return LoadWait{}, err
		}
		return LoadWait{Mode: "constant", Seconds: seconds}, nil
	case "traffic.between":
		minSeconds, err := parsePositionalFloatArgument(arguments, 0)
		if err != nil {
			return LoadWait{}, err
		}
		maxSeconds, err := parsePositionalFloatArgument(arguments, 1)
		if err != nil {
			return LoadWait{}, err
		}
		return LoadWait{Mode: "between", MinSeconds: minSeconds, MaxSeconds: maxSeconds}, nil
	case "traffic.pacing":
		seconds := topologyNamedNumberArgument(arguments, "seconds_per_iteration")
		if seconds == 0 {
			var err error
			seconds, err = parsePositionalFloatArgument(arguments, 0)
			if err != nil {
				return LoadWait{}, err
			}
		}
		return LoadWait{Mode: "pacing", Seconds: seconds}, nil
	default:
		return LoadWait{}, fmt.Errorf("invalid suite topology: unsupported wait helper %q", call)
	}
}

func parseLoadThresholdList(value string, contextState loadPlanContext) ([]LoadThreshold, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}
	inner, err := bracketContents(trimmed)
	if err != nil {
		return nil, err
	}
	items := splitTopLevel(inner)
	thresholds := make([]LoadThreshold, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if threshold, ok := contextState.thresholds[item]; ok {
			thresholds = append(thresholds, threshold)
			continue
		}
		invocation, ok, err := parseTopologyInvocation(item)
		if err != nil {
			return nil, err
		}
		if !ok || canonicalTrafficCall(invocation.Call) != "traffic.threshold" {
			return nil, fmt.Errorf("invalid suite topology: unsupported threshold expression %q", item)
		}
		threshold, err := parseLoadThreshold(invocation.Args)
		if err != nil {
			return nil, err
		}
		thresholds = append(thresholds, threshold)
	}
	return thresholds, nil
}

func parseLoadThresholdListInline(value string) ([]LoadThreshold, error) {
	contextState := loadPlanContext{thresholds: map[string]LoadThreshold{}}
	return parseLoadThresholdList(value, contextState)
}

func parseLoadThreshold(arguments string) (LoadThreshold, error) {
	metric := parsePositionalStringArgument(arguments, 0)
	op := parsePositionalStringArgument(arguments, 1)
	value, err := parsePositionalFloatArgument(arguments, 2)
	if err != nil {
		return LoadThreshold{}, err
	}
	if metric == "" || op == "" {
		return LoadThreshold{}, fmt.Errorf("invalid suite topology: traffic.threshold requires metric, operator, and value")
	}
	return LoadThreshold{
		Metric:  metric,
		Op:      op,
		Value:   value,
		Sampler: topologyNamedStringArgument(arguments, "sampler"),
	}, nil
}

func parseLoadStage(arguments string) (LoadStage, error) {
	durationValue := firstNonEmpty(topologyNamedStringArgument(arguments, "duration"), parsePositionalStringArgument(arguments, 0))
	if durationValue == "" {
		return LoadStage{}, fmt.Errorf("invalid suite topology: traffic.stage requires duration")
	}
	duration, err := time.ParseDuration(durationValue)
	if err != nil {
		return LoadStage{}, fmt.Errorf("invalid suite topology: invalid traffic stage duration %q", durationValue)
	}
	return LoadStage{
		Duration:  duration,
		Users:     int(topologyNamedNumberArgument(arguments, "users")),
		SpawnRate: topologyNamedNumberArgument(arguments, "spawn_rate"),
		Stop:      topologyNamedBoolArgument(arguments, "stop"),
	}, nil
}

func parseLoadStages(value string, contextState loadPlanContext) ([]LoadStage, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}

	invocation, ok, err := parseTopologyInvocation(trimmed)
	if err != nil {
		return nil, err
	}
	if !ok || canonicalTrafficCall(invocation.Call) != "traffic.stages" {
		return nil, fmt.Errorf("invalid suite topology: traffic.plan shape must use traffic.stages(...)")
	}

	inner, err := bracketContents(strings.TrimSpace(invocation.Args))
	if err != nil {
		return nil, err
	}
	items := splitTopLevel(inner)
	stages := make([]LoadStage, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if stage, ok := contextState.stages[item]; ok {
			stages = append(stages, stage)
			continue
		}
		stageInvocation, ok, err := parseTopologyInvocation(item)
		if err != nil {
			return nil, err
		}
		if !ok || canonicalTrafficCall(stageInvocation.Call) != "traffic.stage" {
			return nil, fmt.Errorf("invalid suite topology: unsupported stage expression %q", item)
		}
		stage, err := parseLoadStage(stageInvocation.Args)
		if err != nil {
			return nil, err
		}
		stages = append(stages, stage)
	}
	return stages, nil
}

func parseLoadTaskList(value string, contextState loadPlanContext) ([]LoadTask, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, fmt.Errorf("invalid suite topology: traffic.user must declare tasks")
	}
	inner, err := bracketContents(trimmed)
	if err != nil {
		return nil, err
	}
	items := splitTopLevel(inner)
	tasks := make([]LoadTask, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if task, ok := contextState.tasks[item]; ok {
			tasks = append(tasks, task)
			continue
		}
		invocation, ok, err := parseTopologyInvocation(item)
		if err != nil {
			return nil, err
		}
		if !ok || canonicalTrafficCall(invocation.Call) != "traffic.task" {
			return nil, fmt.Errorf("invalid suite topology: unsupported task expression %q", item)
		}
		task, err := parseLoadTask("", invocation.Args, contextState)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func parseLoadUserList(value string, contextState loadPlanContext) ([]LoadUser, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, fmt.Errorf("invalid suite topology: traffic.plan must declare users")
	}
	inner, err := bracketContents(trimmed)
	if err != nil {
		return nil, err
	}
	items := splitTopLevel(inner)
	users := make([]LoadUser, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if user, ok := contextState.users[item]; ok {
			users = append(users, user)
			continue
		}
		invocation, ok, err := parseTopologyInvocation(item)
		if err != nil {
			return nil, err
		}
		if !ok || canonicalTrafficCall(invocation.Call) != "traffic.user" {
			return nil, fmt.Errorf("invalid suite topology: unsupported user expression %q", item)
		}
		user, err := parseLoadUser("", invocation.Args, contextState)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, nil
}

func parseStringMapLiteral(value string) (map[string]string, error) {
	literal, err := parseLiteralValue(value)
	if err != nil {
		return nil, err
	}
	object, ok := literal.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid suite topology: expected a string map literal")
	}
	result := make(map[string]string, len(object))
	for key, item := range object {
		result[key] = fmt.Sprint(item)
	}
	return result, nil
}

func parseJSONLiteral(value string) (string, error) {
	literal, err := parseLiteralValue(value)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(literal)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func parseLiteralValue(value string) (any, error) {
	parser := literalParser{source: strings.TrimSpace(value)}
	parsed, err := parser.parseValue()
	if err != nil {
		return nil, fmt.Errorf("invalid suite topology: %w", err)
	}
	parser.skipWhitespace()
	if !parser.done() {
		return nil, fmt.Errorf("invalid suite topology: trailing literal content %q", parser.remaining())
	}
	return parsed, nil
}

type literalParser struct {
	source string
	index  int
}

func (p *literalParser) parseValue() (any, error) {
	p.skipWhitespace()
	if p.done() {
		return nil, fmt.Errorf("empty literal")
	}

	switch p.peek() {
	case '"':
		return p.parseString()
	case '{':
		return p.parseObject()
	case '[':
		return p.parseList()
	default:
		if isLiteralNumberStart(p.peek()) {
			return p.parseNumber()
		}
		return p.parseIdentifier()
	}
}

func (p *literalParser) parseString() (string, error) {
	start := p.index
	p.index++
	escaped := false
	for !p.done() {
		ch := p.source[p.index]
		p.index++
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == '"' {
			value, err := strconv.Unquote(p.source[start:p.index])
			if err != nil {
				return "", err
			}
			return value, nil
		}
	}
	return "", fmt.Errorf("unterminated string literal")
}

func (p *literalParser) parseObject() (map[string]any, error) {
	result := make(map[string]any)
	p.index++
	for {
		p.skipWhitespace()
		if p.done() {
			return nil, fmt.Errorf("unterminated object literal")
		}
		if p.peek() == '}' {
			p.index++
			return result, nil
		}

		key, err := p.parseObjectKey()
		if err != nil {
			return nil, err
		}
		p.skipWhitespace()
		if p.done() || p.peek() != ':' {
			return nil, fmt.Errorf("expected ':' after object key")
		}
		p.index++
		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		result[key] = value

		p.skipWhitespace()
		if p.done() {
			return nil, fmt.Errorf("unterminated object literal")
		}
		if p.peek() == ',' {
			p.index++
			continue
		}
		if p.peek() == '}' {
			p.index++
			return result, nil
		}
		return nil, fmt.Errorf("expected ',' or '}' in object literal")
	}
}

func (p *literalParser) parseObjectKey() (string, error) {
	p.skipWhitespace()
	if p.done() {
		return "", fmt.Errorf("missing object key")
	}
	if p.peek() == '"' {
		return p.parseString()
	}
	identifier, err := p.readIdentifier()
	if err != nil {
		return "", err
	}
	return identifier, nil
}

func (p *literalParser) parseList() ([]any, error) {
	items := make([]any, 0)
	p.index++
	for {
		p.skipWhitespace()
		if p.done() {
			return nil, fmt.Errorf("unterminated list literal")
		}
		if p.peek() == ']' {
			p.index++
			return items, nil
		}
		item, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		items = append(items, item)

		p.skipWhitespace()
		if p.done() {
			return nil, fmt.Errorf("unterminated list literal")
		}
		if p.peek() == ',' {
			p.index++
			continue
		}
		if p.peek() == ']' {
			p.index++
			return items, nil
		}
		return nil, fmt.Errorf("expected ',' or ']' in list literal")
	}
}

func (p *literalParser) parseNumber() (any, error) {
	start := p.index
	if p.peek() == '-' {
		p.index++
	}
	for !p.done() && (p.peek() == '.' || isASCIIDigit(rune(p.peek()))) {
		p.index++
	}
	raw := p.source[start:p.index]
	if strings.Contains(raw, ".") {
		return strconv.ParseFloat(raw, 64)
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, err
	}
	return value, nil
}

func (p *literalParser) parseIdentifier() (any, error) {
	identifier, err := p.readIdentifier()
	if err != nil {
		return nil, err
	}
	switch identifier {
	case "True", "true":
		return true, nil
	case "False", "false":
		return false, nil
	case "None", "null":
		return nil, nil
	default:
		return identifier, nil
	}
}

func (p *literalParser) readIdentifier() (string, error) {
	start := p.index
	for !p.done() {
		ch := rune(p.peek())
		if !isTopologyIdentifierPart(ch, p.index == start) {
			break
		}
		p.index++
	}
	if start == p.index {
		return "", fmt.Errorf("expected identifier")
	}
	return p.source[start:p.index], nil
}

func (p *literalParser) skipWhitespace() {
	for !p.done() {
		switch p.peek() {
		case ' ', '\t', '\r', '\n':
			p.index++
		default:
			return
		}
	}
}

func (p *literalParser) done() bool {
	return p.index >= len(p.source)
}

func (p *literalParser) peek() byte {
	return p.source[p.index]
}

func (p *literalParser) remaining() string {
	if p.done() {
		return ""
	}
	return p.source[p.index:]
}

func bracketContents(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) < 2 || trimmed[0] != '[' || trimmed[len(trimmed)-1] != ']' {
		return "", fmt.Errorf("invalid suite topology: expected a list expression, got %q", value)
	}
	return strings.TrimSpace(trimmed[1 : len(trimmed)-1]), nil
}

func parsePositionalStringArgument(arguments string, index int) string {
	part := positionalArgument(arguments, index)
	if part == "" {
		return ""
	}
	resolved, err := unquoteTopologyString(part)
	if err != nil {
		return ""
	}
	return resolved
}

func parsePositionalFloatArgument(arguments string, index int) (float64, error) {
	part := strings.TrimSpace(positionalArgument(arguments, index))
	if part == "" {
		return 0, fmt.Errorf("missing positional argument %d", index)
	}
	value, err := strconv.ParseFloat(part, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid numeric argument %q", part)
	}
	return value, nil
}

func positionalArgument(arguments string, index int) string {
	position := 0
	for _, part := range splitTopLevel(arguments) {
		if _, _, ok := splitNamedArgument(part); ok {
			continue
		}
		if position == index {
			return strings.TrimSpace(part)
		}
		position++
	}
	return ""
}

func topologyNamedNumberArgument(arguments string, keys ...string) float64 {
	value := strings.TrimSpace(topologyNamedArgument(arguments, keys...))
	if value == "" {
		return 0
	}
	number, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	return number
}

func topologyNamedBoolArgument(arguments string, keys ...string) bool {
	value := strings.TrimSpace(topologyNamedArgument(arguments, keys...))
	switch value {
	case "True", "true":
		return true
	default:
		return false
	}
}

func validateLoadSpec(spec *LoadSpec) error {
	if spec == nil {
		return fmt.Errorf("invalid suite topology: missing traffic spec")
	}

	parsedTarget, err := url.Parse(spec.Target)
	if err != nil || parsedTarget.Scheme == "" || parsedTarget.Host == "" {
		return fmt.Errorf("invalid suite topology: traffic target %q must be an absolute URL", spec.Target)
	}
	if len(spec.Users) == 0 {
		return fmt.Errorf("invalid suite topology: traffic plan %q must declare at least one user", spec.PlanPath)
	}
	if len(spec.Stages) == 0 {
		return fmt.Errorf("invalid suite topology: traffic plan %q must declare at least one stage", spec.PlanPath)
	}
	if len(spec.Stages) > maxLoadStages {
		return fmt.Errorf("invalid suite topology: traffic plan %q exceeds the %d stage safety limit", spec.PlanPath, maxLoadStages)
	}

	totalDuration := time.Duration(0)
	maxUsers := 0
	for _, stage := range spec.Stages {
		if stage.Duration <= 0 {
			return fmt.Errorf("invalid suite topology: traffic plan %q contains a non-positive stage duration", spec.PlanPath)
		}
		totalDuration += stage.Duration
		if stage.Users > maxUsers {
			maxUsers = stage.Users
		}
	}
	if totalDuration > maxLoadDuration {
		return fmt.Errorf("invalid suite topology: traffic plan %q exceeds the %s duration safety limit", spec.PlanPath, maxLoadDuration)
	}
	if maxUsers > maxLoadUsers {
		return fmt.Errorf("invalid suite topology: traffic plan %q exceeds the %d user safety limit", spec.PlanPath, maxLoadUsers)
	}

	totalTasks := 0
	for userIndex := range spec.Users {
		if spec.Users[userIndex].Weight <= 0 {
			spec.Users[userIndex].Weight = 1
		}
		if len(spec.Users[userIndex].Tasks) == 0 {
			return fmt.Errorf("invalid suite topology: traffic user %q does not declare any tasks", spec.Users[userIndex].Name)
		}
		totalTasks += len(spec.Users[userIndex].Tasks)
		if totalTasks > maxLoadTasks {
			return fmt.Errorf("invalid suite topology: traffic plan %q exceeds the %d task safety limit", spec.PlanPath, maxLoadTasks)
		}
		for taskIndex := range spec.Users[userIndex].Tasks {
			if spec.Users[userIndex].Tasks[taskIndex].Weight <= 0 {
				spec.Users[userIndex].Tasks[taskIndex].Weight = 1
			}
			if spec.Users[userIndex].Tasks[taskIndex].Request.Method == "" || spec.Users[userIndex].Tasks[taskIndex].Request.Path == "" {
				return fmt.Errorf("invalid suite topology: traffic task %q must declare a request method and path", spec.Users[userIndex].Tasks[taskIndex].Name)
			}
		}
		if spec.Users[userIndex].Wait.Mode == "" {
			if spec.Variant == "traffic.constant_pacing" {
				spec.Users[userIndex].Wait = LoadWait{Mode: "pacing", Seconds: 1}
			} else {
				spec.Users[userIndex].Wait = LoadWait{Mode: "constant", Seconds: 0}
			}
		}
	}

	if spec.Variant == "traffic.constant_throughput" && spec.RequestsPerS <= 0 {
		spec.RequestsPerS = float64(max(1, maxUsers))
	}
	if spec.Variant == "traffic.open_model" && spec.ArrivalRate <= 0 {
		spec.ArrivalRate = float64(max(1, maxUsers))
	}
	if spec.RequestsPerS > maxLoadRequestRate {
		return fmt.Errorf("invalid suite topology: traffic plan %q exceeds the %.0f requests-per-second safety limit", spec.PlanPath, maxLoadRequestRate)
	}
	if spec.ArrivalRate > maxLoadRequestRate {
		return fmt.Errorf("invalid suite topology: traffic plan %q exceeds the %.0f arrivals-per-second safety limit", spec.PlanPath, maxLoadRequestRate)
	}

	return nil
}

func isLiteralNumberStart(ch byte) bool {
	return ch == '-' || (ch >= '0' && ch <= '9')
}

func isTopologyIdentifierPart(ch rune, first bool) bool {
	if first {
		return ch == '_' || isASCIIAlpha(ch)
	}
	return ch == '_' || isASCIIAlpha(ch) || isASCIIDigit(ch)
}

func max(left, right int) int {
	if left > right {
		return left
	}
	return right
}
