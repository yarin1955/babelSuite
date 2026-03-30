package support

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/babelsuite/babelsuite/pkg/apiclient"
	"github.com/babelsuite/babelsuite/pkg/localconfig"
)

type Runtime struct {
	Stdout io.Writer
	Stderr io.Writer
	Store  *localconfig.Store
}

type GlobalOptions struct {
	Server     string
	Output     string
	ConfigPath string
}

func (r *Runtime) Fail(err error) {
	if err == nil {
		return
	}
	_, _ = fmt.Fprintf(r.Stderr, "error: %s\n", err)
}

func (r *Runtime) AuthenticatedClient(opts GlobalOptions) (*apiclient.Client, *localconfig.Config, error) {
	cfg, err := r.Store.Load()
	if err != nil {
		return nil, nil, err
	}

	token := FirstNonEmpty(strings.TrimSpace(os.Getenv("BABELSUITE_TOKEN")), cfg.Token)
	if token == "" {
		return nil, nil, errors.New("no active session; run `babelctl login` first")
	}

	client := apiclient.New(FirstNonEmpty(opts.Server, cfg.Server), token)
	return client, cfg, nil
}

func ExtractGlobalOptions(args []string) (GlobalOptions, []string, error) {
	opts := GlobalOptions{}
	rest := make([]string, 0, len(args))

	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--server":
			index++
			if index >= len(args) {
				return opts, nil, errors.New("--server requires a value")
			}
			opts.Server = args[index]
		case "--output", "-o":
			index++
			if index >= len(args) {
				return opts, nil, errors.New("--output requires a value")
			}
			opts.Output = args[index]
		case "--config":
			index++
			if index >= len(args) {
				return opts, nil, errors.New("--config requires a value")
			}
			opts.ConfigPath = args[index]
		default:
			rest = append(rest, args[index])
		}
	}

	return opts, rest, nil
}

func NewFlagSet(name string, stderr io.Writer, usage func()) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = usage
	return flags
}

func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
