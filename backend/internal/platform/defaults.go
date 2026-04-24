package platform

import "time"

const defaultPlatformDescription = "Physical-layer configuration for BabelSuite execution agents, OCI catalog sources, and shared platform credentials."

func DefaultSettings() PlatformSettings {
	indexedAt := time.Now().UTC().Add(-15 * time.Minute)
	updatedAt := time.Now().UTC()

	return PlatformSettings{
		Mode:        "local",
		Description: defaultPlatformDescription,
		Agents: []ExecutionAgent{
			{
				AgentID:       "local-docker",
				Name:          "Local Docker",
				Type:          "local",
				Description:   "Uses the host machine's native Docker socket with zero extra configuration.",
				Enabled:       true,
				Default:       true,
				Status:        "Ready",
				RoutingTags:   []string{"default", "local"},
				DockerSocket:  "/var/run/docker.sock",
				APISIXSidecar: defaultAPISIXSidecar(),
			},
			{
				AgentID:       "remote-agent",
				Name:          "Remote Worker Pool",
				Type:          "remote-agent",
				Description:   "Dispatches suite steps to a remote worker service for isolated or heavier workloads.",
				Enabled:       false,
				Default:       false,
				Status:        "Disconnected",
				RoutingTags:   []string{"high-memory"},
				HostURL:       "http://agent.internal.company.com:8091",
				TLSCert:       "vault://kv/platform/remote-agent/cert",
				TLSKey:        "vault://kv/platform/remote-agent/key",
				APISIXSidecar: defaultAPISIXSidecar(),
			},
			{
				AgentID:             "kubernetes-burst",
				Name:                "Kubernetes Burst Pool",
				Type:                "kubernetes",
				Description:         "Launches ephemeral pods in Kubernetes when suites need isolated or GPU-enabled compute.",
				Enabled:             false,
				Default:             false,
				Status:              "Standby",
				RoutingTags:         []string{"gpu-enabled", "ci-only"},
				KubeconfigPath:      "/etc/babelsuite/kubeconfig",
				TargetNamespace:     "babelsuite-runs",
				ServiceAccountToken: "vault://kv/platform/k8s/token",
				APISIXSidecar:       defaultAPISIXSidecar(),
			},
		},
		Registries: []OCIRegistry{
			{
				RegistryID:      "catalog-primary",
				Name:            "Local Zot",
				Provider:        "Zot",
				RegistryURL:       "http://localhost:5000",
				Username:          "",
				Secret:            "",
				RepositoryScope:   "*",
				AllowLocalNetwork: true,
				SyncStatus:        "Indexed",
				LastSyncedAt:    &indexedAt,
			},
		},
		Secrets: SecretsConfig{
			Provider:       "vault",
			VaultAddress:   "https://vault.internal.company.com",
			VaultNamespace: "platform",
			VaultRole:      "babelsuite-runners",
			SecretPrefix:   "kv/platform",
			GlobalOverrides: []GlobalOverride{
				{
					Key:         "HTTPS_PROXY",
					Value:       "http://proxy.internal.company.com:8080",
					Description: "Corporate outbound proxy applied to every suite run.",
					Sensitive:   false,
				},
				{
					Key:         "INTERNAL_ROOT_CA",
					Value:       "-----BEGIN CERTIFICATE-----\\nMIIF...\\n-----END CERTIFICATE-----",
					Description: "Root certificate injected into suite containers for internal TLS trust.",
					Sensitive:   true,
				},
			},
		},
		UpdatedAt: updatedAt,
	}
}

func defaultAPISIXSidecar() APISIXSidecarConfig {
	return APISIXSidecarConfig{
		Image:           "apache/apisix:latest",
		ConfigMountPath: "/usr/local/apisix/conf/apisix.yaml",
		ListenPort:      9080,
		AdminPort:       9180,
		Capabilities:    []string{"rest", "soap", "graphql", "grpc", "async", "kafka", "mqtt", "websocket", "sse", "tcp", "udp", "amqp", "nats", "webhook"},
	}
}
