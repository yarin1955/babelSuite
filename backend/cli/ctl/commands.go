package babelctl

import (
	"context"

	"github.com/babelsuite/babelsuite/cli/ctl/authcmd"
	"github.com/babelsuite/babelsuite/cli/ctl/catalogcmd"
	"github.com/babelsuite/babelsuite/cli/ctl/createcmd"
	"github.com/babelsuite/babelsuite/cli/ctl/envcmd"
	"github.com/babelsuite/babelsuite/cli/ctl/internal/support"
	"github.com/babelsuite/babelsuite/cli/ctl/profilescmd"
	"github.com/babelsuite/babelsuite/cli/ctl/runscmd"
	"github.com/babelsuite/babelsuite/cli/ctl/suitescmd"
)

type rootCommand struct {
	name        string
	aliases     []string
	usage       string
	description string
	run         func(context.Context, *Runner, support.GlobalOptions, []string) int
}

type rootCommandGroup struct {
	title    string
	commands []rootCommand
}

func (r *Runner) commandGroups() []rootCommandGroup {
	return []rootCommandGroup{
		{
			title: "Session",
			commands: []rootCommand{
				{
					name:        "login",
					usage:       "login",
					description: "Sign in and persist a local session",
					run: func(ctx context.Context, r *Runner, opts support.GlobalOptions, args []string) int {
						return authcmd.RunLogin(ctx, r.runtime(), opts, args)
					},
				},
				{
					name:        "logout",
					usage:       "logout",
					description: "Clear the saved session token",
					run: func(_ context.Context, r *Runner, opts support.GlobalOptions, _ []string) int {
						return authcmd.RunLogout(r.runtime(), opts)
					},
				},
				{
					name:        "whoami",
					usage:       "whoami",
					description: "Show the current signed-in user",
					run: func(ctx context.Context, r *Runner, opts support.GlobalOptions, _ []string) int {
						return authcmd.RunWhoAmI(ctx, r.runtime(), opts)
					},
				},
			},
		},
		{
			title: "Suites",
			commands: []rootCommand{
				{
					name:        "catalog",
					usage:       "catalog list | inspect <package>",
					description: "Browse packages discovered from configured registries",
					run: func(ctx context.Context, r *Runner, opts support.GlobalOptions, args []string) int {
						return catalogcmd.Run(ctx, r.runtime(), opts, args)
					},
				},
				{
					name:        "create",
					usage:       "create <name> [destination]",
					description: "Create a starter suite template on disk",
					run: func(ctx context.Context, r *Runner, opts support.GlobalOptions, args []string) int {
						return createcmd.Run(ctx, r.runtime(), opts, args)
					},
				},
				{
					name:        "suites",
					usage:       "suites list | get <suite> | inspect <suite>",
					description: "List and inspect available suites",
					run: func(ctx context.Context, r *Runner, opts support.GlobalOptions, args []string) int {
						return suitescmd.Run(ctx, r.runtime(), opts, args)
					},
				},
				{
					name:        "profiles",
					usage:       "profiles list <suite>",
					description: "List suite-scoped launch profiles",
					run: func(ctx context.Context, r *Runner, opts support.GlobalOptions, args []string) int {
						return profilescmd.Run(ctx, r.runtime(), opts, args)
					},
				},
			},
		},
		{
			title: "Executions",
			commands: []rootCommand{
				{
					name:        "runs",
					usage:       "runs list | get <id>",
					description: "List executions and inspect execution details",
					run: func(ctx context.Context, r *Runner, opts support.GlobalOptions, args []string) int {
						return runscmd.Run(ctx, r.runtime(), opts, args)
					},
				},
				{
					name:        "run",
					usage:       "run <suite|repository[:tag]> [--profile <profile.yaml>]",
					description: "Create a new execution",
					run: func(ctx context.Context, r *Runner, opts support.GlobalOptions, args []string) int {
						return runscmd.RunCreateExecution(ctx, r.runtime(), opts, args)
					},
				},
				{
					name:        "fork",
					usage:       "fork <suite|repository[:tag]> [destination]",
					description: "Write an inspectable suite to a local folder",
					run: func(ctx context.Context, r *Runner, opts support.GlobalOptions, args []string) int {
						return runscmd.RunFork(ctx, r.runtime(), opts, args)
					},
				},
			},
		},
		{
			title: "Environments",
			commands: []rootCommand{
				{
					name:        "environments",
					aliases:     []string{"envs"},
					usage:       "environments list | reap <id> | reap-all",
					description: "List and clean up managed environments",
					run: func(ctx context.Context, r *Runner, opts support.GlobalOptions, args []string) int {
						return envcmd.Run(ctx, r.runtime(), opts, args)
					},
				},
			},
		},
		{
			title: "System",
			commands: []rootCommand{
				{
					name:        "version",
					usage:       "version",
					description: "Print the CLI version label",
					run: func(_ context.Context, r *Runner, _ support.GlobalOptions, _ []string) int {
						_, _ = r.stdout.Write([]byte("babelctl dev\n"))
						return 0
					},
				},
			},
		},
	}
}

func (r *Runner) findCommand(name string) (rootCommand, bool) {
	for _, group := range r.commandGroups() {
		for _, command := range group.commands {
			if command.name == name {
				return command, true
			}
			for _, alias := range command.aliases {
				if alias == name {
					return command, true
				}
			}
		}
	}
	return rootCommand{}, false
}
