package suites

import (
	"strings"
	"testing"
)

func TestDefaultDispatcherRules_REST(t *testing.T) {
	// C1: REST transport, GET method
	t.Parallel()
	surface := APISurface{Protocol: "http"}
	op := APIOperation{Method: "GET", Name: "/orders"}
	metadata := MockOperationMetadata{}
	got := defaultDispatcherRules("suite-1", surface, op, metadata)
	if !strings.Contains(got, "GET") {
		t.Errorf("expected GET in output, got: %s", got)
	}
	if !strings.Contains(got, "/orders") {
		t.Errorf("expected /orders in output, got: %s", got)
	}
}

func TestDefaultDispatcherRules_GRPC(t *testing.T) {
	// C2: gRPC transport
	t.Parallel()
	surface := APISurface{Protocol: "grpc"}
	op := APIOperation{Name: "/orders.OrderService/GetOrder"}
	metadata := MockOperationMetadata{}
	got := defaultDispatcherRules("suite-1", surface, op, metadata)
	if !strings.Contains(got, "gRPC") {
		t.Errorf("expected 'gRPC' in output, got: %s", got)
	}
}

func TestDefaultDispatcherRules_SOAP(t *testing.T) {
	// C8: SOAP protocol, POST method
	t.Parallel()
	surface := APISurface{Protocol: "SOAP"}
	op := APIOperation{Method: "POST", Name: "/soap/service"}
	metadata := MockOperationMetadata{}
	got := defaultDispatcherRules("suite-1", surface, op, metadata)
	if !strings.Contains(got, "SOAP") {
		t.Errorf("expected 'SOAP' in output, got: %s", got)
	}
	if !strings.Contains(got, "POST") {
		t.Errorf("expected 'POST' in output, got: %s", got)
	}
}

func TestDefaultDispatcherRules_GRPCSAlias(t *testing.T) {
	// C9: grpcs alias normalised to grpc — output identical to gRPC case
	t.Parallel()
	surface := APISurface{Protocol: "grpcs"}
	op := APIOperation{Name: "/svc/Method"}
	metadata := MockOperationMetadata{}
	got := defaultDispatcherRules("suite-1", surface, op, metadata)
	if !strings.Contains(got, "gRPC") {
		t.Errorf("expected 'gRPC' in output for grpcs alias, got: %s", got)
	}
}

func TestDefaultDispatcherRules_EmptyMethodNormalisedToPost(t *testing.T) {
	// C12: empty method → normalised to POST in REST output
	t.Parallel()
	surface := APISurface{Protocol: "http"}
	op := APIOperation{Method: "", Name: "/rpc"}
	metadata := MockOperationMetadata{}
	got := defaultDispatcherRules("suite-1", surface, op, metadata)
	if !strings.Contains(got, "POST") {
		t.Errorf("expected 'POST' in output for empty method, got: %s", got)
	}
}

func TestDefaultDispatcherRules_ResolverURLOverride(t *testing.T) {
	// C13: custom ResolverURL → resolver path is the URL's path component
	t.Parallel()
	surface := APISurface{Protocol: "http"}
	op := APIOperation{Method: "GET", Name: "/items"}
	metadata := MockOperationMetadata{ResolverURL: "http://resolver.internal/custom/path"}
	got := defaultDispatcherRules("suite-1", surface, op, metadata)
	if !strings.Contains(got, "/custom/path") {
		t.Errorf("expected /custom/path in output, got: %s", got)
	}
}

func TestDefaultDispatcherRules_NameBasedPublicPath(t *testing.T) {
	// C14: operation.Name starts with / → used as public path verbatim
	t.Parallel()
	surface := APISurface{Protocol: "http"}
	op := APIOperation{Method: "GET", Name: "/orders/{id}"}
	metadata := MockOperationMetadata{}
	got := defaultDispatcherRules("suite-1", surface, op, metadata)
	if !strings.Contains(got, "/orders/{id}") {
		t.Errorf("expected /orders/{id} in output, got: %s", got)
	}
}

func TestSanitizeIdentifier_AllSymbolsProducesValidPath(t *testing.T) {
	// Proof Q1: sanitizeIdentifier("///") returns "", but publicPathForOperation still returns "/"
	t.Parallel()
	id := sanitizeIdentifier("///")
	if id != "" {
		t.Errorf("expected empty string for all-symbol ID, got %q", id)
	}
	op := APIOperation{ID: "///"}
	path := publicPathForOperation(op)
	if !strings.HasPrefix(path, "/") {
		t.Errorf("publicPathForOperation must return /-prefixed path, got %q", path)
	}
	if path == "" {
		t.Error("publicPathForOperation must not return empty string")
	}
}
