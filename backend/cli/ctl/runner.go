package babelctl

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/babelsuite/babelsuite/cli/ctl/authcmd"
	"github.com/babelsuite/babelsuite/cli/ctl/catalogcmd"
	"github.com/babelsuite/babelsuite/cli/ctl/envcmd"
	"github.com/babelsuite/babelsuite/cli/ctl/internal/support"
	"github.com/babelsuite/babelsuite/cli/ctl/profilescmd"
	"github.com/babelsuite/babelsuite/cli/ctl/runscmd"
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

	switch remaining[0] {
	case "-h", "--help", "help":
		r.printRootHelp()
		return 0
	case "version":
		_, _ = fmt.Fprintln(r.stdout, "babelctl dev")
		return 0
	case "login":
		return authcmd.RunLogin(ctx, r.runtime(), opts, remaining[1:])
	case "logout":
		return authcmd.RunLogout(r.runtime(), opts)
	case "whoami":
		return authcmd.RunWhoAmI(ctx, r.runtime(), opts)
	case "catalog":
		return catalogcmd.Run(ctx, r.runtime(), opts, remaining[1:])
	case "profiles":
		return profilescmd.Run(ctx, r.runtime(), opts, remaining[1:])
	case "runs":
		return runscmd.Run(ctx, r.runtime(), opts, remaining[1:])
	case "run":
		return runscmd.RunCreateExecution(ctx, r.runtime(), opts, remaining[1:])
	case "fork":
		return runscmd.RunFork(ctx, r.runtime(), opts, remaining[1:])
	case "environments":
		return envcmd.Run(ctx, r.runtime(), opts, remaining[1:])
	default:
		r.fail(fmt.Errorf("unknown command %q", remaining[0]))
		r.printRootHelp()
		return 1
	}
}

func (r *Runner) printRootHelp() {
	_, _ = fmt.Fprintf(r.stdout, `babelctl connects to the BabelSuite API and persists a local session.

Usage:
  babelctl [--server <url>] [--output text|json] <command>

Commands:
  login                         Sign in and persist a local session
  logout                        Clear the saved session token
  whoami                        Show the current signed-in user
  catalog list                  List packages discovered from configured registries
  catalog inspect <package>     Show catalog metadata for a package
  profiles list <suite>         List suite-scoped launch profiles
  runs list                     List executions
  runs get <id>                 Show execution details and events
  run <suite|repository[:tag]>  Create a new execution
  fork <suite|repository[:tag]> [destination]
                                Write an inspectable suite to a local folder
  environments list             List managed environments
  environments reap <id>        Reap one environment
  environments reap-all         Reap all managed environments
  version                       Print the CLI version label

Config:
  %s
`, r.store.Path())
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
