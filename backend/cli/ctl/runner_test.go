package babelctl

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunnerHelpIsGeneratedFromCommandRegistry(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)

	status := runner.Run(context.Background(), []string{"help"})
	if status != 0 {
		t.Fatalf("expected exit 0, got %d", status)
	}

	output := stdout.String()
	for _, expected := range []string{
		"Session:",
		"Suites:",
		"Executions:",
		"Environments:",
		"System:",
		"create <name> [destination]",
		"suites list | get <suite> | inspect <suite>",
		"environments list | reap <id> | reap-all",
		"aliases: envs",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected help output to contain %q, got:\n%s", expected, output)
		}
	}
}

func TestRunnerDispatchesSystemVersionCommand(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)

	status := runner.Run(context.Background(), []string{"version"})
	if status != 0 {
		t.Fatalf("expected exit 0, got %d", status)
	}
	if got := stdout.String(); got != "babelctl dev\n" {
		t.Fatalf("unexpected version output %q", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", stderr.String())
	}
}

func TestRunnerUnknownCommandPrintsErrorAndHelp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)

	status := runner.Run(context.Background(), []string{"nope"})
	if status != 1 {
		t.Fatalf("expected exit 1, got %d", status)
	}
	if !strings.Contains(stderr.String(), `unknown command "nope"`) {
		t.Fatalf("expected unknown command error, got %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Commands:") {
		t.Fatalf("expected help output after unknown command, got %q", stdout.String())
	}
}
