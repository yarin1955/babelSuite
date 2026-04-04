package authcmd

import (
	"context"
	"fmt"

	"github.com/babelsuite/babelsuite/cli/ctl/internal/support"
	"github.com/babelsuite/babelsuite/pkg/apiclient"
)

func RunLogin(ctx context.Context, rt *support.Runtime, opts support.GlobalOptions, args []string) int {
	flags := support.NewFlagSet("login", rt.Stderr, func() {
		_, _ = fmt.Fprintln(rt.Stderr, "Usage: babelctl login --email <email> --password <password>")
	})

	var email string
	var password string
	flags.StringVar(&email, "email", "", "account email")
	flags.StringVar(&password, "password", "", "account password")
	if err := flags.Parse(args); err != nil {
		return 1
	}
	if support.FirstNonEmpty(email) == "" || password == "" {
		flags.Usage()
		return 1
	}

	cfg, err := rt.Store.Load()
	if err != nil {
		rt.Fail(err)
		return 1
	}

	client := apiclient.New(support.FirstNonEmpty(opts.Server, cfg.Server), "")
	response, err := client.SignIn(ctx, email, password)
	if err != nil {
		rt.Fail(err)
		return 1
	}

	cfg.Server = client.BaseURL
	cfg.Token = response.Token
	cfg.Email = response.User.Email
	cfg.FullName = response.User.FullName
	cfg.Workspace = response.Workspace.Name
	cfg.ExpiresAt = response.ExpiresAt
	if err := rt.Store.Save(cfg); err != nil {
		rt.Fail(err)
		return 1
	}

	if opts.Output == "json" {
		_ = support.PrintJSON(rt.Stdout, map[string]any{
			"server":    cfg.Server,
			"user":      response.User,
			"workspace": response.Workspace,
			"expiresAt": response.ExpiresAt,
			"config":    rt.Store.Path(),
		})
		return 0
	}

	support.PrintKeyValues(rt.Stdout, [][2]string{
		{"Signed in as", support.FirstNonEmpty(response.User.FullName, response.User.Email)},
		{"Email", response.User.Email},
		{"Workspace", response.Workspace.Name},
		{"Server", cfg.Server},
		{"Config", rt.Store.Path()},
		{"Session expires", support.FormatTime(response.ExpiresAt)},
	})
	return 0
}

func RunLogout(rt *support.Runtime, opts support.GlobalOptions) int {
	cfg, err := rt.Store.Load()
	if err != nil {
		rt.Fail(err)
		return 1
	}

	if support.FirstNonEmpty(opts.Server) != "" {
		cfg.Server = apiclient.New(opts.Server, "").BaseURL
		if err := rt.Store.Save(cfg); err != nil {
			rt.Fail(err)
			return 1
		}
	}

	if err := rt.Store.ClearSession(); err != nil {
		rt.Fail(err)
		return 1
	}

	if opts.Output == "json" {
		_ = support.PrintJSON(rt.Stdout, map[string]any{
			"server":    cfg.Server,
			"loggedOut": true,
		})
		return 0
	}

	_, _ = fmt.Fprintln(rt.Stdout, "Signed out.")
	return 0
}

func RunWhoAmI(ctx context.Context, rt *support.Runtime, opts support.GlobalOptions) int {
	client, cfg, err := rt.AuthenticatedClient(opts)
	if err != nil {
		rt.Fail(err)
		return 1
	}

	user, err := client.Me(ctx)
	if err != nil {
		rt.Fail(err)
		return 1
	}

	if opts.Output == "json" {
		_ = support.PrintJSON(rt.Stdout, map[string]any{
			"user":      user,
			"workspace": cfg.Workspace,
			"server":    client.BaseURL,
			"expiresAt": cfg.ExpiresAt,
		})
		return 0
	}

	support.PrintKeyValues(rt.Stdout, [][2]string{
		{"Full name", support.FirstNonEmpty(user.FullName, "-")},
		{"Email", user.Email},
		{"Username", user.Username},
		{"Admin", support.FormatBool(user.IsAdmin)},
		{"Workspace", support.FirstNonEmpty(cfg.Workspace, "-")},
		{"Server", client.BaseURL},
		{"Session expires", support.FormatTime(cfg.ExpiresAt)},
	})
	return 0
}
