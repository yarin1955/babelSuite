package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/babelsuite/babelsuite/cli/internal/session"
	"github.com/babelsuite/babelsuite/pkg/api"
	babelclient "github.com/babelsuite/babelsuite/pkg/client"
)

const defaultServerURL = "http://localhost:8090"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printRootUsage()
		return nil
	}

	switch args[0] {
	case "auth":
		return runAuth(args[1:])
	case "catalog":
		return runCatalog(args[1:])
	case "runs":
		return runRuns(args[1:])
	case "agents":
		return runAgents(args[1:])
	case "help", "-h", "--help":
		printRootUsage()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runAuth(args []string) error {
	if len(args) == 0 {
		printAuthUsage()
		return nil
	}

	switch args[0] {
	case "login":
		fs := flag.NewFlagSet("auth login", flag.ContinueOnError)
		server := fs.String("server", "", "")
		username := fs.String("username", "", "")
		password := fs.String("password", "", "")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *username == "" || *password == "" {
			return errors.New("username and password are required")
		}

		client, state, err := newClient(*server, false)
		if err != nil {
			return err
		}
		resp, err := client.Login(context.Background(), api.LoginRequest{
			Username: *username,
			Password: *password,
		})
		if err != nil {
			return err
		}

		state.Server = client.BaseURL()
		state.Token = resp.Token
		if err := session.Save(state); err != nil {
			return err
		}

		return printJSON(map[string]any{
			"server": client.BaseURL(),
			"user":   resp.User,
			"org":    resp.Org,
		})
	case "me":
		fs := flag.NewFlagSet("auth me", flag.ContinueOnError)
		server := fs.String("server", "", "")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}

		client, _, err := newClient(*server, true)
		if err != nil {
			return err
		}
		user, err := client.Me(context.Background())
		if err != nil {
			return err
		}
		return printJSON(user)
	case "logout":
		state, err := session.Load()
		if err != nil {
			return err
		}
		state.Token = ""
		if err := session.Save(state); err != nil {
			return err
		}
		return printJSON(map[string]any{"logged_out": true, "server": state.Server})
	default:
		return fmt.Errorf("unknown auth command %q", args[0])
	}
}

func runCatalog(args []string) error {
	if len(args) == 0 {
		printCatalogUsage()
		return nil
	}

	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("catalog list", flag.ContinueOnError)
		server := fs.String("server", "", "")
		search := fs.String("q", "", "")
		page := fs.Int("page", 1, "")
		pageSize := fs.Int("page-size", 20, "")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}

		client, _, err := newClient(*server, true)
		if err != nil {
			return err
		}
		resp, err := client.ListCatalog(context.Background(), *search, *page, *pageSize)
		if err != nil {
			return err
		}
		return printJSON(resp)
	case "get":
		fs := flag.NewFlagSet("catalog get", flag.ContinueOnError)
		server := fs.String("server", "", "")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("catalog get requires a package id")
		}

		client, _, err := newClient(*server, true)
		if err != nil {
			return err
		}
		pkg, err := client.GetPackage(context.Background(), fs.Arg(0))
		if err != nil {
			return err
		}
		return printJSON(pkg)
	default:
		return fmt.Errorf("unknown catalog command %q", args[0])
	}
}

func runRuns(args []string) error {
	if len(args) == 0 {
		printRunsUsage()
		return nil
	}

	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("runs list", flag.ContinueOnError)
		server := fs.String("server", "", "")
		page := fs.Int("page", 1, "")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}

		client, _, err := newClient(*server, true)
		if err != nil {
			return err
		}
		resp, err := client.ListRuns(context.Background(), *page)
		if err != nil {
			return err
		}
		return printJSON(resp)
	case "get":
		fs := flag.NewFlagSet("runs get", flag.ContinueOnError)
		server := fs.String("server", "", "")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("runs get requires a run id")
		}

		client, _, err := newClient(*server, true)
		if err != nil {
			return err
		}
		run, err := client.GetRun(context.Background(), fs.Arg(0))
		if err != nil {
			return err
		}
		return printJSON(run)
	case "start":
		fs := flag.NewFlagSet("runs start", flag.ContinueOnError)
		server := fs.String("server", "", "")
		profile := fs.String("profile", "", "")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("runs start requires a package id")
		}

		client, _, err := newClient(*server, true)
		if err != nil {
			return err
		}
		run, err := client.CreateRun(context.Background(), fs.Arg(0), *profile)
		if err != nil {
			return err
		}
		return printJSON(run)
	case "cancel":
		fs := flag.NewFlagSet("runs cancel", flag.ContinueOnError)
		server := fs.String("server", "", "")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("runs cancel requires a run id")
		}

		client, _, err := newClient(*server, true)
		if err != nil {
			return err
		}
		if err := client.CancelRun(context.Background(), fs.Arg(0)); err != nil {
			return err
		}
		return printJSON(map[string]any{"run_id": fs.Arg(0), "canceled": true})
	case "steps":
		fs := flag.NewFlagSet("runs steps", flag.ContinueOnError)
		server := fs.String("server", "", "")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("runs steps requires a run id")
		}

		client, _, err := newClient(*server, true)
		if err != nil {
			return err
		}
		steps, err := client.ListSteps(context.Background(), fs.Arg(0))
		if err != nil {
			return err
		}
		return printJSON(steps)
	case "logs":
		fs := flag.NewFlagSet("runs logs", flag.ContinueOnError)
		server := fs.String("server", "", "")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 2 {
			return errors.New("runs logs requires a run id and step id")
		}

		client, _, err := newClient(*server, true)
		if err != nil {
			return err
		}
		logs, err := client.HistoryLogs(context.Background(), fs.Arg(0), fs.Arg(1))
		if err != nil {
			return err
		}
		return printJSON(logs)
	default:
		return fmt.Errorf("unknown runs command %q", args[0])
	}
}

func runAgents(args []string) error {
	if len(args) == 0 {
		printAgentsUsage()
		return nil
	}

	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("agents list", flag.ContinueOnError)
		server := fs.String("server", "", "")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}

		client, _, err := newClient(*server, true)
		if err != nil {
			return err
		}
		agents, err := client.ListAgents(context.Background())
		if err != nil {
			return err
		}
		return printJSON(agents)
	case "get":
		fs := flag.NewFlagSet("agents get", flag.ContinueOnError)
		server := fs.String("server", "", "")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("agents get requires an agent id")
		}

		client, _, err := newClient(*server, true)
		if err != nil {
			return err
		}
		agent, err := client.GetAgent(context.Background(), fs.Arg(0))
		if err != nil {
			return err
		}
		return printJSON(agent)
	case "create":
		fs := flag.NewFlagSet("agents create", flag.ContinueOnError)
		server := fs.String("server", "", "")
		name := fs.String("name", "", "")
		backend := fs.String("backend", "docker", "")
		platform := fs.String("platform", "", "")
		targetName := fs.String("target-name", "", "")
		targetURL := fs.String("target-url", "", "")
		capacity := fs.Int("capacity", 1, "")
		paused := fs.Bool("paused", false, "")
		var labels labelFlags
		fs.Var(&labels, "label", "")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *name == "" {
			return errors.New("agents create requires --name")
		}

		client, _, err := newClient(*server, true)
		if err != nil {
			return err
		}
		resp, err := client.CreateAgent(context.Background(), api.CreateAgentRequest{
			Name:              *name,
			DesiredBackend:    *backend,
			DesiredPlatform:   *platform,
			DesiredTargetName: *targetName,
			DesiredTargetURL:  *targetURL,
			Capacity:          *capacity,
			Labels:            labels.Map(),
			NoSchedule:        *paused,
		})
		if err != nil {
			return err
		}
		return printJSON(resp)
	case "update":
		fs := flag.NewFlagSet("agents update", flag.ContinueOnError)
		server := fs.String("server", "", "")
		name := fs.String("name", "", "")
		backend := fs.String("backend", "", "")
		platform := fs.String("platform", "", "")
		targetName := fs.String("target-name", "", "")
		targetURL := fs.String("target-url", "", "")
		capacity := fs.Int("capacity", 0, "")
		paused := fs.String("paused", "", "")
		clearLabels := fs.Bool("clear-labels", false, "")
		var labels labelFlags
		fs.Var(&labels, "label", "")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("agents update requires an agent id")
		}

		visited := visitedFlags(fs)
		req := api.UpdateAgentRequest{}

		if visited["name"] {
			value := *name
			req.Name = &value
		}
		if visited["backend"] {
			value := *backend
			req.DesiredBackend = &value
		}
		if visited["platform"] {
			value := *platform
			req.DesiredPlatform = &value
		}
		if visited["target-name"] {
			value := *targetName
			req.DesiredTargetName = &value
		}
		if visited["target-url"] {
			value := *targetURL
			req.DesiredTargetURL = &value
		}
		if visited["capacity"] {
			if *capacity <= 0 {
				return errors.New("capacity must be greater than zero")
			}
			value := *capacity
			req.Capacity = &value
		}
		if visited["paused"] {
			value, err := strconv.ParseBool(*paused)
			if err != nil {
				return errors.New("paused must be true or false")
			}
			req.NoSchedule = &value
		}
		if *clearLabels {
			req.Labels = map[string]string{}
		} else if visited["label"] {
			req.Labels = labels.Map()
		}

		client, _, err := newClient(*server, true)
		if err != nil {
			return err
		}
		agent, err := client.UpdateAgent(context.Background(), fs.Arg(0), req)
		if err != nil {
			return err
		}
		return printJSON(agent)
	case "delete":
		fs := flag.NewFlagSet("agents delete", flag.ContinueOnError)
		server := fs.String("server", "", "")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("agents delete requires an agent id")
		}

		client, _, err := newClient(*server, true)
		if err != nil {
			return err
		}
		if err := client.DeleteAgent(context.Background(), fs.Arg(0)); err != nil {
			return err
		}
		return printJSON(map[string]any{"agent_id": fs.Arg(0), "deleted": true})
	default:
		return fmt.Errorf("unknown agents command %q", args[0])
	}
}

func newClient(serverFlag string, requireToken bool) (*babelclient.Client, session.State, error) {
	state, err := session.Load()
	if err != nil {
		return nil, session.State{}, err
	}

	server := firstNonEmpty(serverFlag, os.Getenv("BABELSUITE_SERVER"), state.Server)
	if server == "" {
		server = defaultServerURL
	}
	token := firstNonEmpty(os.Getenv("BABELSUITE_TOKEN"), state.Token)
	if requireToken && token == "" {
		return nil, state, errors.New("no auth token found; run `babel auth login`")
	}

	client := babelclient.New(server,
		babelclient.WithToken(token),
		babelclient.WithHTTPClient(&http.Client{Timeout: 30 * time.Second}),
	)
	return client, state, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func printRootUsage() {
	fmt.Println("babel <command>")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  auth login|me|logout")
	fmt.Println("  catalog list|get")
	fmt.Println("  runs list|get|start|cancel|steps|logs")
	fmt.Println("  agents list|get|create|update|delete")
}

func printAuthUsage() {
	fmt.Println("babel auth login --username <user> --password <pass> [--server <url>]")
	fmt.Println("babel auth me [--server <url>]")
	fmt.Println("babel auth logout")
}

func printCatalogUsage() {
	fmt.Println("babel catalog list [--q <search>] [--page <n>] [--page-size <n>] [--server <url>]")
	fmt.Println("babel catalog get <package-id> [--server <url>]")
}

func printRunsUsage() {
	fmt.Println("babel runs list [--page <n>] [--server <url>]")
	fmt.Println("babel runs get <run-id> [--server <url>]")
	fmt.Println("babel runs start <package-id> [--profile <name>] [--server <url>]")
	fmt.Println("babel runs cancel <run-id> [--server <url>]")
	fmt.Println("babel runs steps <run-id> [--server <url>]")
	fmt.Println("babel runs logs <run-id> <step-id> [--server <url>]")
}

func printAgentsUsage() {
	fmt.Println("babel agents list [--server <url>]")
	fmt.Println("babel agents get <agent-id> [--server <url>]")
	fmt.Println("babel agents create --name <name> [--backend docker|kubernetes|local] [--platform <target>] [--target-name <name>] [--target-url <url>] [--capacity <n>] [--paused] [--label key=value] [--server <url>]")
	fmt.Println("babel agents update <agent-id> [--name <name>] [--backend docker|kubernetes|local|\"\"] [--platform <target>] [--target-name <name>] [--target-url <url>] [--capacity <n>] [--paused true|false] [--label key=value] [--clear-labels] [--server <url>]")
	fmt.Println("babel agents delete <agent-id> [--server <url>]")
}

type labelFlags []string

func (l *labelFlags) String() string {
	return ""
}

func (l *labelFlags) Set(value string) error {
	if _, _, ok := splitLabel(value); !ok {
		return errors.New("label must be in key=value form")
	}
	*l = append(*l, value)
	return nil
}

func (l labelFlags) Map() map[string]string {
	if len(l) == 0 {
		return nil
	}
	out := make(map[string]string, len(l))
	for _, item := range l {
		key, value, ok := splitLabel(item)
		if !ok {
			continue
		}
		out[key] = value
	}
	return out
}

func splitLabel(value string) (string, string, bool) {
	for i := 0; i < len(value); i++ {
		if value[i] == '=' {
			key := value[:i]
			if key == "" {
				return "", "", false
			}
			return key, value[i+1:], true
		}
	}
	return "", "", false
}

func visitedFlags(fs *flag.FlagSet) map[string]bool {
	visited := map[string]bool{}
	fs.Visit(func(f *flag.Flag) {
		visited[f.Name] = true
	})
	return visited
}
