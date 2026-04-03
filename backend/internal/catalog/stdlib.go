package catalog

func SeedStdlibPackages() []Package {
	return clonePackages(seedStdlib())
}

func seedStdlib() []Package {
	return []Package{
		{
			ID:          "stdlib-runtime",
			Kind:        "stdlib",
			Title:       "@babelsuite/runtime",
			Repository:  "localhost:5000/babelsuite/runtime",
			Owner:       "BabelSuite Stdlib",
			Provider:    "Stdlib",
			Version:     "0.9.0",
			Tags:        []string{"0.9.0", "0.8.3", "latest"},
			Description: "Core suite runtime module exposing container.run/create/get, mock.serve, service.wiremock/prism/custom, plus script and scenario entry points for topology orchestration.",
			Modules:     []string{"container lifecycle", "native mocks", "compat services", "topology steps", "debug helpers"},
			Status:      "Official",
			Score:       97,
			PullCommand: "babelctl run localhost:5000/babelsuite/runtime:0.9.0",
			ForkCommand: "babelctl fork localhost:5000/babelsuite/runtime:0.9.0 ./stdlib-runtime",
			Inspectable: false,
		},
		{
			ID:          "stdlib-postgres",
			Kind:        "stdlib",
			Title:       "@babelsuite/postgres",
			Repository:  "localhost:5000/babelsuite/postgres",
			Owner:       "BabelSuite Stdlib",
			Provider:    "Stdlib",
			Version:     "1.4.0",
			Tags:        []string{"1.4.0", "1.3.2", "latest"},
			Description: "Pre-registered Starlark module for opinionated Postgres provisioning with strict connection URL contracts.",
			Modules:     []string{"typed api contract", "health checks", "auto scripts"},
			Status:      "Official",
			Score:       98,
			PullCommand: "babelctl run localhost:5000/babelsuite/postgres:1.4.0",
			ForkCommand: "babelctl fork localhost:5000/babelsuite/postgres:1.4.0 ./stdlib-postgres",
			Inspectable: false,
		},
		{
			ID:          "stdlib-kafka",
			Kind:        "stdlib",
			Title:       "@babelsuite/kafka",
			Repository:  "localhost:5000/babelsuite/kafka",
			Owner:       "BabelSuite Stdlib",
			Provider:    "Stdlib",
			Version:     "1.2.3",
			Tags:        []string{"1.2.3", "1.2.2", "latest"},
			Description: "Typed Kafka module that creates brokers, topics, and address outputs without leaking Docker wiring into suite authorship.",
			Modules:     []string{"topics", "bootstrap address", "consumer groups"},
			Status:      "Official",
			Score:       96,
			PullCommand: "babelctl run localhost:5000/babelsuite/kafka:1.2.3",
			ForkCommand: "babelctl fork localhost:5000/babelsuite/kafka:1.2.3 ./stdlib-kafka",
			Inspectable: false,
		},
	}
}
