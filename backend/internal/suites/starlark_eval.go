package suites

import (
	"fmt"
	"strings"
	"sync"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

var starlarkFileOptions = &syntax.FileOptions{
	Set:             true,
	GlobalReassign:  true,
	Recursion:       true,
	TopLevelControl: true,
	While:           true,
}

type starlarkNode struct {
	id              string
	name            string
	explicitName    bool
	kind            string
	variant         string
	image           string
	file            string
	ref             string
	plan            string
	target          string
	rps             float64
	arrivalRate     float64
	after           []*starlarkNode
	resetMocks      []*starlarkNode
	onFailure       []*starlarkNode
	continueOnFail  bool
	evaluation      *StepEvaluation
	exports         []ArtifactExport
	order           int
}

func (n *starlarkNode) String() string        { return n.name }
func (n *starlarkNode) Type() string          { return "babelsuite.Node" }
func (n *starlarkNode) Freeze()               {}
func (n *starlarkNode) Truth() starlark.Bool  { return starlark.True }
func (n *starlarkNode) Hash() (uint32, error) { return 0, fmt.Errorf("node is not hashable") }

type starlarkRegistry struct {
	mu    sync.Mutex
	nodes []*starlarkNode
}

func (r *starlarkRegistry) register(n *starlarkNode) {
	r.mu.Lock()
	n.order = len(r.nodes)
	r.nodes = append(r.nodes, n)
	r.mu.Unlock()
}

func evalStarlarkTopology(suiteStar string) (nodes []rawTopologyNode, retErr error) {
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("starlark evaluation panicked: %v", r)
		}
	}()

	reg := &starlarkRegistry{}

	predeclared, err := buildRuntimePredeclared(reg)
	if err != nil {
		return nil, err
	}

	thread := &starlark.Thread{
		Name: "suite.star",
		Load: func(t *starlark.Thread, module string) (starlark.StringDict, error) {
			return resolveStarlarkModule(module, reg)
		},
	}

	globals, err := starlark.ExecFileOptions(starlarkFileOptions, thread, "suite.star", suiteStar, predeclared)
	if err != nil {
		return nil, fmt.Errorf("starlark: %w", err)
	}

	if len(reg.nodes) == 0 {
		return nil, fmt.Errorf("starlark: no topology nodes registered")
	}

	assignIDs(reg, globals)
	return buildRawNodes(reg), nil
}

func buildRuntimePredeclared(reg *starlarkRegistry) (starlark.StringDict, error) {
	runtimeModule, err := buildRuntimeModule(reg)
	if err != nil {
		return nil, err
	}
	return starlark.StringDict{
		"service": runtimeModule["service"],
		"task":    runtimeModule["task"],
		"test":    runtimeModule["test"],
		"traffic": runtimeModule["traffic"],
		"suite":   runtimeModule["suite"],
	}, nil
}

func resolveStarlarkModule(module string, reg *starlarkRegistry) (starlark.StringDict, error) {
	if module == "@babelsuite/runtime" {
		return buildRuntimeModule(reg)
	}
	if strings.HasPrefix(module, "@babelsuite/") {
		return starlark.StringDict{}, nil
	}
	return nil, fmt.Errorf("unknown module %q", module)
}

func buildRuntimeModule(reg *starlarkRegistry) (starlark.StringDict, error) {
	service := &starlarkNamespace{
		reg: reg,
		methods: map[string]starlarkBuilderFunc{
			"run":      buildNodeFunc(reg, "service.run"),
			"mock":     buildNodeFunc(reg, "service.mock"),
			"wiremock": buildNodeFunc(reg, "service.wiremock"),
			"prism":    buildNodeFunc(reg, "service.prism"),
			"custom":   buildNodeFunc(reg, "service.custom"),
		},
	}
	task := &starlarkNamespace{
		reg: reg,
		methods: map[string]starlarkBuilderFunc{
			"run": buildNodeFunc(reg, "task.run"),
		},
	}
	test := &starlarkNamespace{
		reg: reg,
		methods: map[string]starlarkBuilderFunc{
			"run": buildNodeFunc(reg, "test.run"),
		},
	}
	trafficVariants := []string{
		"smoke", "baseline", "stress", "spike", "soak",
		"scalability", "step", "wave", "staged",
		"constant_throughput", "constant_pacing", "open_model",
	}
	trafficMethods := make(map[string]starlarkBuilderFunc, len(trafficVariants))
	for _, v := range trafficVariants {
		trafficMethods[v] = buildNodeFunc(reg, "traffic."+v)
	}
	traffic := &starlarkNamespace{reg: reg, methods: trafficMethods}
	suite := &starlarkNamespace{
		reg: reg,
		methods: map[string]starlarkBuilderFunc{
			"run": buildNodeFunc(reg, "suite.run"),
		},
	}

	return starlark.StringDict{
		"service": service,
		"task":    task,
		"test":    test,
		"traffic": traffic,
		"suite":   suite,
	}, nil
}

type starlarkBuilderFunc func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error)

type starlarkNamespace struct {
	reg     *starlarkRegistry
	methods map[string]starlarkBuilderFunc
}

func (ns *starlarkNamespace) String() string        { return "babelsuite.Namespace" }
func (ns *starlarkNamespace) Type() string          { return "babelsuite.Namespace" }
func (ns *starlarkNamespace) Freeze()               {}
func (ns *starlarkNamespace) Truth() starlark.Bool  { return starlark.True }
func (ns *starlarkNamespace) Hash() (uint32, error) { return 0, fmt.Errorf("namespace is not hashable") }

func (ns *starlarkNamespace) Attr(name string) (starlark.Value, error) {
	fn, ok := ns.methods[name]
	if !ok {
		return nil, nil
	}
	return starlark.NewBuiltin(name, fn), nil
}

func (ns *starlarkNamespace) AttrNames() []string {
	names := make([]string, 0, len(ns.methods))
	for k := range ns.methods {
		names = append(names, k)
	}
	return names
}

func buildNodeFunc(reg *starlarkRegistry, variant string) starlarkBuilderFunc {
	kind, _ := topologyKind(variant)
	if kind == "" {
		kind = variant
	}
	return func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		node := &starlarkNode{
			kind:    kind,
			variant: variant,
		}

		var expectExit *int
		var expectLogs []string
		var failOnLogs []string

		for _, kwarg := range kwargs {
			key := string(kwarg[0].(starlark.String))
			val := kwarg[1]

			switch key {
			case "name", "name_or_id", "id":
				s, ok := starlark.AsString(val)
				if !ok {
					return nil, fmt.Errorf("%s: %s must be a string", variant, key)
				}
				node.name = strings.TrimSpace(s)
				node.explicitName = true

			case "image":
				s, ok := starlark.AsString(val)
				if !ok {
					return nil, fmt.Errorf("%s: image must be a string", variant)
				}
				node.image = strings.TrimSpace(s)

			case "file":
				s, ok := starlark.AsString(val)
				if !ok {
					return nil, fmt.Errorf("%s: file must be a string", variant)
				}
				node.file = strings.TrimSpace(s)

			case "ref":
				s, ok := starlark.AsString(val)
				if !ok {
					return nil, fmt.Errorf("%s: ref must be a string", variant)
				}
				node.ref = strings.TrimSpace(s)

			case "after":
				deps, err := extractNodeList(val, variant, "after")
				if err != nil {
					return nil, err
				}
				node.after = deps

			case "reset_mocks":
				deps, err := extractNodeList(val, variant, "reset_mocks")
				if err != nil {
					return nil, err
				}
				node.resetMocks = deps

			case "on_failure":
				deps, err := extractNodeList(val, variant, "on_failure")
				if err != nil {
					return nil, err
				}
				node.onFailure = deps

			case "continue_on_failure":
				b, ok := val.(starlark.Bool)
				if !ok {
					return nil, fmt.Errorf("%s: continue_on_failure must be a bool", variant)
				}
				node.continueOnFail = bool(b)

			case "plan":
				s, ok := starlark.AsString(val)
				if !ok {
					return nil, fmt.Errorf("%s: plan must be a string", variant)
				}
				node.plan = strings.TrimSpace(s)

			case "target":
				s, ok := starlark.AsString(val)
				if !ok {
					return nil, fmt.Errorf("%s: target must be a string", variant)
				}
				node.target = strings.TrimSpace(s)

			case "rps", "target_rps":
				if f, ok := starlark.AsFloat(val); ok {
					node.rps = f
				}

			case "arrival_rate":
				if f, ok := starlark.AsFloat(val); ok {
					node.arrivalRate = f
				}

			case "expect_exit":
				code, ok := val.(starlark.Int)
				if !ok {
					return nil, fmt.Errorf("%s: expect_exit must be an int", variant)
				}
				n64, _ := code.Int64()
				n := int(n64)
				expectExit = &n

			case "expect_logs":
				matchers, err := extractStringOrList(val, variant, "expect_logs")
				if err != nil {
					return nil, err
				}
				expectLogs = matchers

			case "fail_on_logs":
				matchers, err := extractStringOrList(val, variant, "fail_on_logs")
				if err != nil {
					return nil, err
				}
				failOnLogs = matchers
			}
		}

		if expectExit != nil || len(expectLogs) > 0 || len(failOnLogs) > 0 {
			node.evaluation = &StepEvaluation{
				ExpectExit: expectExit,
				ExpectLogs: expectLogs,
				FailOnLogs: failOnLogs,
			}
		}

		reg.register(node)
		return node, nil
	}
}

func extractNodeList(val starlark.Value, call, param string) ([]*starlarkNode, error) {
	list, ok := val.(*starlark.List)
	if !ok {
		return nil, fmt.Errorf("%s: %s must be a list", call, param)
	}
	out := make([]*starlarkNode, 0, list.Len())
	for i := 0; i < list.Len(); i++ {
		elem := list.Index(i)
		node, ok := elem.(*starlarkNode)
		if !ok {
			return nil, fmt.Errorf("%s: %s elements must be node references", call, param)
		}
		out = append(out, node)
	}
	return out, nil
}

func extractStringOrList(val starlark.Value, call, param string) ([]string, error) {
	if s, ok := starlark.AsString(val); ok {
		if s != "" {
			return []string{s}, nil
		}
		return nil, nil
	}
	list, ok := val.(*starlark.List)
	if !ok {
		return nil, fmt.Errorf("%s: %s must be a string or list of strings", call, param)
	}
	out := make([]string, 0, list.Len())
	for i := 0; i < list.Len(); i++ {
		s, ok := starlark.AsString(list.Index(i))
		if !ok {
			return nil, fmt.Errorf("%s: %s list elements must be strings", call, param)
		}
		if s != "" {
			out = append(out, s)
		}
	}
	return out, nil
}

func assignIDs(reg *starlarkRegistry, globals starlark.StringDict) {
	nodeToVar := make(map[*starlarkNode]string, len(reg.nodes))
	for varName, val := range globals {
		if node, ok := val.(*starlarkNode); ok {
			nodeToVar[node] = varName
		}
	}

	for _, node := range reg.nodes {
		varName := nodeToVar[node]

		if node.explicitName {
			node.id = node.name
		} else if varName != "" {
			node.id = varName
			node.name = varName
		} else {
			node.id = fmt.Sprintf("node_%d", node.order)
			node.name = node.id
		}
	}
}

func buildStarlarkArguments(node *starlarkNode) string {
	var parts []string
	if node.plan != "" {
		parts = append(parts, fmt.Sprintf(`plan="%s"`, node.plan))
	}
	if node.target != "" {
		parts = append(parts, fmt.Sprintf(`target="%s"`, node.target))
	}
	if node.rps > 0 {
		parts = append(parts, fmt.Sprintf(`rps=%g`, node.rps))
	}
	if node.arrivalRate > 0 {
		parts = append(parts, fmt.Sprintf(`arrival_rate=%g`, node.arrivalRate))
	}
	return strings.Join(parts, ", ")
}

func buildRawNodes(reg *starlarkRegistry) []rawTopologyNode {
	raw := make([]rawTopologyNode, 0, len(reg.nodes))
	for _, node := range reg.nodes {
		rn := rawTopologyNode{
			Assignment:        node.id,
			ID:                node.id,
			Name:              node.name,
			Kind:              node.kind,
			Variant:           node.variant,
			Ref:               node.ref,
			Arguments:         buildStarlarkArguments(node),
			ContinueOnFailure: node.continueOnFail,
			Evaluation:        node.evaluation,
			Exports:           append([]ArtifactExport{}, node.exports...),
			Order:             node.order,
		}

		for _, dep := range node.after {
			if dep.id != "" {
				rn.DependsOn = append(rn.DependsOn, dep.id)
			}
		}
		for _, dep := range node.resetMocks {
			if dep.id != "" {
				rn.ResetMocks = append(rn.ResetMocks, dep.id)
			}
		}
		for _, dep := range node.onFailure {
			if dep.id != "" {
				rn.OnFailure = append(rn.OnFailure, dep.id)
			}
		}

		raw = append(raw, rn)
	}
	return raw
}
