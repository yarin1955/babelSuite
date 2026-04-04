package runscmd

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/babelsuite/babelsuite/cli/ctl/internal/support"
)

func Run(ctx context.Context, rt *support.Runtime, opts support.GlobalOptions, args []string) int {
	if len(args) == 0 || args[0] == "list" {
		return RunList(ctx, rt, opts)
	}
	switch args[0] {
	case "get":
		return RunGet(ctx, rt, opts, args[1:])
	case "-h", "--help", "help":
		_, _ = fmt.Fprintln(rt.Stdout, "Usage: babelctl runs list | get <execution-id>")
		return 0
	default:
		rt.Fail(fmt.Errorf("unknown runs command %q", args[0]))
		return 1
	}
}

func RunList(ctx context.Context, rt *support.Runtime, opts support.GlobalOptions) int {
	client, _, err := rt.AuthenticatedClient(opts)
	if err != nil {
		rt.Fail(err)
		return 1
	}

	runs, err := client.ListRuns(ctx)
	if err != nil {
		rt.Fail(err)
		return 1
	}

	sort.Slice(runs, func(i, j int) bool {
		return runs[i].StartedAt.After(runs[j].StartedAt)
	})

	if opts.Output == "json" {
		_ = support.PrintJSON(rt.Stdout, runs)
		return 0
	}

	rows := make([][]string, 0, len(runs))
	for _, run := range runs {
		rows = append(rows, []string{
			run.ID,
			run.SuiteTitle,
			run.Profile,
			run.Status,
			run.Trigger,
			run.Duration,
			support.FormatTime(run.StartedAt),
		})
	}
	support.PrintTable(rt.Stdout, []string{"ID", "SUITE", "PROFILE", "STATUS", "TRIGGER", "DURATION", "STARTED"}, rows)
	return 0
}

func RunGet(ctx context.Context, rt *support.Runtime, opts support.GlobalOptions, args []string) int {
	if len(args) == 0 {
		rt.Fail(errors.New("runs get requires an execution id"))
		return 1
	}

	client, _, err := rt.AuthenticatedClient(opts)
	if err != nil {
		rt.Fail(err)
		return 1
	}

	run, err := client.GetRun(ctx, args[0])
	if err != nil {
		rt.Fail(err)
		return 1
	}

	if opts.Output == "json" {
		_ = support.PrintJSON(rt.Stdout, run)
		return 0
	}

	support.PrintKeyValues(rt.Stdout, [][2]string{
		{"Execution", run.ID},
		{"Suite", run.Suite.Title},
		{"Repository", run.Suite.Repository},
		{"Profile", run.Profile},
		{"Status", run.Status},
		{"Trigger", run.Trigger},
		{"Author", run.Author},
		{"Branch", run.Branch},
		{"Commit", run.Commit},
		{"Duration", run.Duration},
		{"Started", support.FormatTime(run.StartedAt)},
		{"Updated", support.FormatTime(run.UpdatedAt)},
	})
	_, _ = fmt.Fprintf(rt.Stdout, "\n%s\n\n", run.Message)

	rows := make([][]string, 0, len(run.Events))
	for _, event := range run.Events {
		rows = append(rows, []string{event.Timestamp, event.Source, event.Status, event.Level, event.Text})
	}
	support.PrintTable(rt.Stdout, []string{"TIME", "SOURCE", "STATUS", "LEVEL", "TEXT"}, rows)
	return 0
}

func RunCreateExecution(ctx context.Context, rt *support.Runtime, opts support.GlobalOptions, args []string) int {
	flags := support.NewFlagSet("run", rt.Stderr, func() {
		_, _ = fmt.Fprintln(rt.Stderr, "Usage: babelctl run <suite|repository[:tag]> [--profile <profile.yaml>]")
	})

	var profile string
	flags.StringVar(&profile, "profile", "", "suite-scoped launch profile")
	if err := flags.Parse(args); err != nil {
		return 1
	}
	if flags.NArg() == 0 {
		flags.Usage()
		return 1
	}

	client, _, err := rt.AuthenticatedClient(opts)
	if err != nil {
		rt.Fail(err)
		return 1
	}

	launchSuites, err := client.ListLaunchSuites(ctx)
	if err != nil {
		rt.Fail(err)
		return 1
	}
	target, err := support.ResolveLaunchTarget(flags.Arg(0), launchSuites)
	if err != nil {
		rt.Fail(err)
		return 1
	}

	execution, err := client.CreateRun(ctx, target.ID, profile)
	if err != nil {
		rt.Fail(err)
		return 1
	}

	if opts.Output == "json" {
		_ = support.PrintJSON(rt.Stdout, execution)
		return 0
	}

	support.PrintKeyValues(rt.Stdout, [][2]string{
		{"Created execution", execution.ID},
		{"Suite", execution.SuiteTitle},
		{"Profile", execution.Profile},
		{"Status", execution.Status},
		{"Trigger", execution.Trigger},
	})
	return 0
}

func RunFork(ctx context.Context, rt *support.Runtime, opts support.GlobalOptions, args []string) int {
	flags := support.NewFlagSet("fork", rt.Stderr, func() {
		_, _ = fmt.Fprintln(rt.Stderr, "Usage: babelctl fork <suite|repository[:tag]> [destination] [--force]")
	})

	var force bool
	flags.BoolVar(&force, "force", false, "overwrite existing files")
	if err := flags.Parse(args); err != nil {
		return 1
	}
	if flags.NArg() == 0 {
		flags.Usage()
		return 1
	}

	client, _, err := rt.AuthenticatedClient(opts)
	if err != nil {
		rt.Fail(err)
		return 1
	}

	suites, err := client.ListSuites(ctx)
	if err != nil {
		rt.Fail(err)
		return 1
	}
	suite, err := support.ResolveSuiteTarget(flags.Arg(0), suites)
	if err != nil {
		rt.Fail(err)
		return 1
	}
	fullSuite, err := client.GetSuite(ctx, suite.ID)
	if err != nil {
		rt.Fail(err)
		return 1
	}

	destination := ""
	if flags.NArg() > 1 {
		destination = flags.Arg(1)
	}
	destination = support.FirstNonEmpty(destination, support.DefaultForkDestination(flags.Arg(0), fullSuite))

	written, err := support.WriteSuiteFiles(destination, fullSuite.SourceFiles, force)
	if err != nil {
		rt.Fail(err)
		return 1
	}

	if opts.Output == "json" {
		_ = support.PrintJSON(rt.Stdout, map[string]any{
			"suiteId":     fullSuite.ID,
			"title":       fullSuite.Title,
			"destination": destination,
			"files":       written,
		})
		return 0
	}

	support.PrintKeyValues(rt.Stdout, [][2]string{
		{"Forked suite", fullSuite.Title},
		{"Destination", destination},
		{"Files written", fmt.Sprintf("%d", written)},
	})
	return 0
}
