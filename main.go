package main

import (
	"log"
	"os"

	"github.com/urfave/cli/v2"

	"github.com/babelsuite/babelsuite/pkg/api"
	"github.com/babelsuite/babelsuite/pkg/config"
	"github.com/babelsuite/babelsuite/pkg/engine"
)

func main() {
	app := &cli.App{
		Name:  "babelsuite",
		Usage: "BabelSuite orchestrator for hardware-in-the-loop simulation test suites",
		Commands: []*cli.Command{
			{
				Name:    "run",
				Aliases: []string{"r"},
				Usage:   "Run a Starlark test suite",
				Action: func(c *cli.Context) error {
					file := c.Args().First()
					if file == "" {
						file = "pipeline.star"
					}

					log.Printf("Starting BabelSuite Engine (container execution)...")
					dockerEngine, err := engine.NewDockerEngine()
					if err != nil {
						return err
					}

					log.Printf("Parsing Starlark AST for: %s", file)
					pipeline, err := config.ParseStarlark(file)
					if err != nil {
						return err
					}

					log.Printf("Executing Topologies on ephemeral Docker network...")
					return dockerEngine.Execute(pipeline)
				},
			},
			{
				Name:    "daemon",
				Aliases: []string{"d"},
				Usage:   "Start daemon with UI/API and multiplexed logs",
				Action: func(c *cli.Context) error {
					log.Println("Starting BabelSuite REST API and DAG UI server on :3000")
					return api.Start(":3000")
				},
			},
			{
				Name:    "deploy",
				Aliases: []string{"dp"},
				Usage:   "Deploy environment config (declarative deployment)",
				Action: func(c *cli.Context) error {
					log.Println("Deploying topology configuration... (Simulation environment ready)")
					return nil
				},
			},
			{
				Name:    "publish",
				Aliases: []string{"pub"},
				Usage:   "Publish a local simulator or test suite to the BabelSuite Web Hub",
				Action: func(c *cli.Context) error {
					log.Println("Packaging and publishing simulator to Hub registry...")
					return nil
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
