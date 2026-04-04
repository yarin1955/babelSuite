package envcmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/babelsuite/babelsuite/cli/ctl/internal/support"
	"github.com/babelsuite/babelsuite/pkg/apiclient"
)

func Run(ctx context.Context, rt *support.Runtime, opts support.GlobalOptions, args []string) int {
	if len(args) == 0 || args[0] == "list" {
		return RunList(ctx, rt, opts)
	}
	switch args[0] {
	case "reap":
		return RunReap(ctx, rt, opts, args[1:])
	case "reap-all":
		return RunReapAll(ctx, rt, opts)
	case "-h", "--help", "help":
		_, _ = fmt.Fprintln(rt.Stdout, "Usage: babelctl environments list | reap <sandbox-id> | reap-all")
		return 0
	default:
		rt.Fail(fmt.Errorf("unknown environments command %q", args[0]))
		return 1
	}
}

func RunList(ctx context.Context, rt *support.Runtime, opts support.GlobalOptions) int {
	client, _, err := rt.AuthenticatedClient(opts)
	if err != nil {
		rt.Fail(err)
		return 1
	}

	inventory, err := client.ListEnvironments(ctx)
	if err != nil {
		rt.Fail(err)
		return 1
	}

	if opts.Output == "json" {
		_ = support.PrintJSON(rt.Stdout, inventory)
		return 0
	}
	if len(inventory.Sandboxes) == 0 {
		_, _ = fmt.Fprintln(rt.Stdout, "No managed environments.")
		return 0
	}

	rows := make([][]string, 0, len(inventory.Sandboxes))
	for _, item := range inventory.Sandboxes {
		rows = append(rows, []string{
			item.SandboxID,
			item.Status,
			item.Suite,
			item.Profile,
			fmt.Sprintf("%d", len(item.Containers)),
			fmt.Sprintf("%d", len(item.Networks)),
			fmt.Sprintf("%d", len(item.Volumes)),
			support.FormatBool(item.IsZombie),
			support.FormatBytes(item.ResourceUsage.MemoryBytes),
			support.FormatFloat(item.ResourceUsage.CPUPercent) + "%",
		})
	}
	support.PrintTable(rt.Stdout, []string{"ID", "STATUS", "SUITE", "PROFILE", "CTR", "NET", "VOL", "ZOMBIE", "MEM", "CPU"}, rows)
	return 0
}

func RunReap(ctx context.Context, rt *support.Runtime, opts support.GlobalOptions, args []string) int {
	if len(args) == 0 {
		rt.Fail(errors.New("environments reap requires a sandbox id"))
		return 1
	}

	client, _, err := rt.AuthenticatedClient(opts)
	if err != nil {
		rt.Fail(err)
		return 1
	}
	result, err := client.ReapEnvironment(ctx, args[0])
	if err != nil {
		rt.Fail(err)
		return 1
	}
	return printReapResult(rt, opts, result)
}

func RunReapAll(ctx context.Context, rt *support.Runtime, opts support.GlobalOptions) int {
	client, _, err := rt.AuthenticatedClient(opts)
	if err != nil {
		rt.Fail(err)
		return 1
	}
	result, err := client.ReapAllEnvironments(ctx)
	if err != nil {
		rt.Fail(err)
		return 1
	}
	return printReapResult(rt, opts, result)
}

func printReapResult(rt *support.Runtime, opts support.GlobalOptions, result *apiclient.ReapResult) int {
	if opts.Output == "json" {
		_ = support.PrintJSON(rt.Stdout, result)
		return 0
	}

	support.PrintKeyValues(rt.Stdout, [][2]string{
		{"Scope", result.Scope},
		{"Target", result.Target},
		{"Removed containers", fmt.Sprintf("%d", result.RemovedContainers)},
		{"Removed networks", fmt.Sprintf("%d", result.RemovedNetworks)},
		{"Removed volumes", fmt.Sprintf("%d", result.RemovedVolumes)},
	})
	if len(result.Warnings) > 0 {
		_, _ = fmt.Fprintln(rt.Stdout, "\nWarnings:")
		for _, warning := range result.Warnings {
			_, _ = fmt.Fprintf(rt.Stdout, "- %s\n", warning)
		}
	}
	return 0
}
