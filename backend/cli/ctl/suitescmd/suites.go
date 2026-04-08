package suitescmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/babelsuite/babelsuite/cli/ctl/internal/support"
)

func Run(ctx context.Context, rt *support.Runtime, opts support.GlobalOptions, args []string) int {
	if len(args) == 0 || args[0] == "list" {
		return RunList(ctx, rt, opts)
	}
	switch args[0] {
	case "get", "inspect":
		return RunGet(ctx, rt, opts, args[1:])
	case "-h", "--help", "help":
		_, _ = fmt.Fprintln(rt.Stdout, "Usage: babelctl suites list | get <suite> | inspect <suite>")
		return 0
	default:
		rt.Fail(fmt.Errorf("unknown suites command %q", args[0]))
		return 1
	}
}

func RunList(ctx context.Context, rt *support.Runtime, opts support.GlobalOptions) int {
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

	sort.Slice(suites, func(i, j int) bool {
		return strings.ToLower(suites[i].Title) < strings.ToLower(suites[j].Title)
	})

	if opts.Output == "json" {
		_ = support.PrintJSON(rt.Stdout, suites)
		return 0
	}

	rows := make([][]string, 0, len(suites))
	for _, suite := range suites {
		rows = append(rows, []string{
			suite.ID,
			suite.Title,
			suite.Repository,
			suite.Version,
			fmt.Sprintf("%d", len(suite.Profiles)),
			fmt.Sprintf("%d", len(suite.Modules)),
		})
	}
	support.PrintTable(rt.Stdout, []string{"ID", "TITLE", "REPOSITORY", "VERSION", "PROFILES", "MODULES"}, rows)
	return 0
}

func RunGet(ctx context.Context, rt *support.Runtime, opts support.GlobalOptions, args []string) int {
	if len(args) == 0 {
		rt.Fail(fmt.Errorf("suites get requires a suite id or repository"))
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
	suiteSummary, err := support.ResolveSuiteTarget(args[0], suites)
	if err != nil {
		rt.Fail(err)
		return 1
	}

	suite, err := client.GetSuite(ctx, suiteSummary.ID)
	if err != nil {
		rt.Fail(err)
		return 1
	}

	if opts.Output == "json" {
		_ = support.PrintJSON(rt.Stdout, suite)
		return 0
	}

	support.PrintKeyValues(rt.Stdout, [][2]string{
		{"Suite", suite.Title},
		{"ID", suite.ID},
		{"Repository", suite.Repository},
		{"Owner", support.FirstNonEmpty(suite.Owner, "-")},
		{"Provider", support.FirstNonEmpty(suite.Provider, "-")},
		{"Version", support.FirstNonEmpty(suite.Version, "-")},
		{"Status", support.FirstNonEmpty(suite.Status, "-")},
		{"Profiles", fmt.Sprintf("%d", len(suite.Profiles))},
		{"Modules", fmt.Sprintf("%d", len(suite.Modules))},
		{"Source files", fmt.Sprintf("%d", len(suite.SourceFiles))},
	})

	if strings.TrimSpace(suite.Description) != "" {
		_, _ = fmt.Fprintf(rt.Stdout, "\n%s\n", suite.Description)
	}

	if len(suite.Profiles) > 0 {
		_, _ = fmt.Fprintln(rt.Stdout, "")
		profileRows := make([][]string, 0, len(suite.Profiles))
		for _, profile := range suite.Profiles {
			profileRows = append(profileRows, []string{
				profile.FileName,
				profile.Label,
				support.FormatBool(profile.Default),
				profile.Description,
			})
		}
		support.PrintTable(rt.Stdout, []string{"PROFILE", "LABEL", "DEFAULT", "DESCRIPTION"}, profileRows)
	}

	if len(suite.Modules) > 0 {
		_, _ = fmt.Fprintln(rt.Stdout, "")
		_, _ = fmt.Fprintf(rt.Stdout, "Modules: %s\n", strings.Join(suite.Modules, ", "))
	}

	if len(suite.SourceFiles) > 0 {
		_, _ = fmt.Fprintln(rt.Stdout, "")
		sourceRows := make([][]string, 0, len(suite.SourceFiles))
		for _, file := range suite.SourceFiles {
			sourceRows = append(sourceRows, []string{
				file.Path,
				file.Language,
			})
		}
		support.PrintTable(rt.Stdout, []string{"PATH", "LANGUAGE"}, sourceRows)
	}

	return 0
}
