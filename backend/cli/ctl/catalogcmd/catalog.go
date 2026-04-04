package catalogcmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/babelsuite/babelsuite/cli/ctl/internal/support"
	"github.com/babelsuite/babelsuite/pkg/apiclient"
)

func Run(ctx context.Context, rt *support.Runtime, opts support.GlobalOptions, args []string) int {
	if len(args) == 0 || args[0] == "list" {
		return RunList(ctx, rt, opts, args[1:])
	}
	switch args[0] {
	case "inspect":
		return RunInspect(ctx, rt, opts, args[1:])
	case "-h", "--help", "help":
		_, _ = fmt.Fprintln(rt.Stdout, "Usage: babelctl catalog list | inspect <package>")
		return 0
	default:
		rt.Fail(fmt.Errorf("unknown catalog command %q", args[0]))
		return 1
	}
}

func RunList(ctx context.Context, rt *support.Runtime, opts support.GlobalOptions, args []string) int {
	flags := support.NewFlagSet("catalog list", rt.Stderr, func() {
		_, _ = fmt.Fprintln(rt.Stderr, "Usage: babelctl catalog list [--kind suite|stdlib] [--starred]")
	})

	var kind string
	var starredOnly bool
	flags.StringVar(&kind, "kind", "", "filter by package kind")
	flags.BoolVar(&starredOnly, "starred", false, "show only starred packages")
	if err := flags.Parse(args); err != nil {
		return 1
	}

	client, _, err := rt.AuthenticatedClient(opts)
	if err != nil {
		rt.Fail(err)
		return 1
	}

	packages, err := client.ListCatalog(ctx)
	if err != nil {
		rt.Fail(err)
		return 1
	}

	filteredRows := make([][]string, 0, len(packages))
	filteredPackages := make([]apiclient.CatalogPackage, 0, len(packages))
	for _, item := range packages {
		if kind != "" && !strings.EqualFold(item.Kind, kind) {
			continue
		}
		if starredOnly && !item.Starred {
			continue
		}
		filteredPackages = append(filteredPackages, item)
		star := ""
		if item.Starred {
			star = "*"
		}
		filteredRows = append(filteredRows, []string{
			star,
			item.Kind,
			item.ID,
			item.Title,
			item.Version,
			item.Provider,
			item.Repository,
		})
	}

	if opts.Output == "json" {
		_ = support.PrintJSON(rt.Stdout, filteredPackages)
		return 0
	}

	support.PrintTable(rt.Stdout, []string{"STAR", "KIND", "ID", "TITLE", "VERSION", "PROVIDER", "REPOSITORY"}, filteredRows)
	return 0
}

func RunInspect(ctx context.Context, rt *support.Runtime, opts support.GlobalOptions, args []string) int {
	if len(args) == 0 {
		rt.Fail(errors.New("catalog inspect requires a package id or repository reference"))
		return 1
	}

	client, _, err := rt.AuthenticatedClient(opts)
	if err != nil {
		rt.Fail(err)
		return 1
	}

	packages, err := client.ListCatalog(ctx)
	if err != nil {
		rt.Fail(err)
		return 1
	}
	item, err := support.ResolveCatalogTarget(args[0], packages)
	if err != nil {
		rt.Fail(err)
		return 1
	}

	detail, err := client.GetCatalogPackage(ctx, item.ID)
	if err != nil {
		rt.Fail(err)
		return 1
	}

	if opts.Output == "json" {
		_ = support.PrintJSON(rt.Stdout, detail)
		return 0
	}

	rows := [][2]string{
		{"Title", detail.Title},
		{"ID", detail.ID},
		{"Kind", detail.Kind},
		{"Repository", detail.Repository},
		{"Version", detail.Version},
		{"Provider", detail.Provider},
		{"Owner", detail.Owner},
		{"Status", detail.Status},
		{"Inspectable", support.FormatBool(detail.Inspectable)},
		{"Starred", support.FormatBool(detail.Starred)},
		{"Tags", strings.Join(detail.Tags, ", ")},
		{"Capabilities", strings.Join(detail.Modules, ", ")},
		{"Pull", detail.PullCommand},
		{"Fork", detail.ForkCommand},
	}
	support.PrintKeyValues(rt.Stdout, rows)
	_, _ = fmt.Fprintf(rt.Stdout, "\n%s\n", detail.Description)
	return 0
}
