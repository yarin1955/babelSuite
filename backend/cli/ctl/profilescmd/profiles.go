package profilescmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/babelsuite/babelsuite/cli/babelctl/internal/support"
)

func Run(ctx context.Context, rt *support.Runtime, opts support.GlobalOptions, args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		_, _ = fmt.Fprintln(rt.Stdout, "Usage: babelctl profiles list <suite>")
		return 0
	}
	if args[0] != "list" {
		rt.Fail(fmt.Errorf("unknown profiles command %q", args[0]))
		return 1
	}
	if len(args) < 2 {
		rt.Fail(errors.New("profiles list requires a suite id or repository reference"))
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
	suite, err := support.ResolveSuiteTarget(args[1], suites)
	if err != nil {
		rt.Fail(err)
		return 1
	}

	profiles, err := client.ListProfiles(ctx, suite.ID)
	if err != nil {
		rt.Fail(err)
		return 1
	}

	if opts.Output == "json" {
		_ = support.PrintJSON(rt.Stdout, profiles)
		return 0
	}

	support.PrintKeyValues(rt.Stdout, [][2]string{
		{"Suite", profiles.SuiteTitle},
		{"Repository", profiles.Repository},
		{"Default", support.FirstNonEmpty(profiles.DefaultProfileFileName, "-")},
	})
	_, _ = fmt.Fprintln(rt.Stdout)
	rows := make([][]string, 0, len(profiles.Profiles))
	for _, profile := range profiles.Profiles {
		defaultMark := ""
		if profile.Default {
			defaultMark = "*"
		}
		rows = append(rows, []string{
			defaultMark,
			profile.FileName,
			profile.Name,
			profile.Scope,
			support.FormatBool(profile.Launchable),
			support.FormatTime(profile.UpdatedAt),
		})
	}
	support.PrintTable(rt.Stdout, []string{"DEFAULT", "FILE", "NAME", "SCOPE", "LAUNCHABLE", "UPDATED"}, rows)
	return 0
}
