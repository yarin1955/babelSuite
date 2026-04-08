package babelctl

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/babelsuite/babelsuite/cli/ctl/internal/support"
	"github.com/babelsuite/babelsuite/pkg/localconfig"
)

type Runner struct {
	stdout io.Writer
	stderr io.Writer
	store  *localconfig.Store
}

func NewRunner(stdout, stderr io.Writer) *Runner {
	return &Runner{
		stdout: stdout,
		stderr: stderr,
		store:  localconfig.NewStore(""),
	}
}

func (r *Runner) Run(ctx context.Context, args []string) int {
	opts, remaining, err := support.ExtractGlobalOptions(args)
	if err != nil {
		r.fail(err)
		return 1
	}
	if strings.TrimSpace(opts.ConfigPath) != "" {
		r.store = localconfig.NewStore(opts.ConfigPath)
	}
	if opts.Output == "" {
		opts.Output = "text"
	}
	if opts.Output != "text" && opts.Output != "json" {
		r.fail(fmt.Errorf("unsupported output format %q", opts.Output))
		return 1
	}

	if len(remaining) == 0 {
		r.printRootHelp()
		return 0
	}

	if slices.Contains([]string{"-h", "--help", "help"}, remaining[0]) {
		r.printRootHelp()
		return 0
	}

	command, ok := r.findCommand(remaining[0])
	if !ok {
		r.fail(fmt.Errorf("unknown command %q", remaining[0]))
		r.printRootHelp()
		return 1
	}
	return command.run(ctx, r, opts, remaining[1:])
}

func (r *Runner) printRootHelp() {
	_, _ = fmt.Fprintf(r.stdout, "babelctl connects to the BabelSuite API and persists a local session.\n\n")
	_, _ = fmt.Fprintf(r.stdout, "Usage:\n  babelctl [--server <url>] [--output text|json] <command>\n\n")
	_, _ = fmt.Fprintf(r.stdout, "Commands:\n")

	usageWidth := 0
	for _, group := range r.commandGroups() {
		for _, command := range group.commands {
			if len(command.usage) > usageWidth {
				usageWidth = len(command.usage)
			}
		}
	}
	for _, group := range r.commandGroups() {
		_, _ = fmt.Fprintf(r.stdout, "  %s:\n", group.title)
		for _, command := range group.commands {
			_, _ = fmt.Fprintf(r.stdout, "    %-*s  %s\n", usageWidth, command.usage, command.description)
			if len(command.aliases) > 0 {
				_, _ = fmt.Fprintf(r.stdout, "    %-*s  aliases: %s\n", usageWidth, "", strings.Join(command.aliases, ", "))
			}
		}
		_, _ = fmt.Fprintln(r.stdout)
	}

	_, _ = fmt.Fprintf(r.stdout, "Config:\n  %s\n", r.store.Path())
}

func (r *Runner) runtime() *support.Runtime {
	return &support.Runtime{
		Stdout: r.stdout,
		Stderr: r.stderr,
		Store:  r.store,
	}
}

func (r *Runner) fail(err error) {
	if err == nil {
		return
	}
	_, _ = fmt.Fprintf(r.stderr, "error: %s\n", err)
}
