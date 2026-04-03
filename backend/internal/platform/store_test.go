package platform

import "testing"

func TestDefaultSettingsIncludeAPISIXSidecarOnAgents(t *testing.T) {
	settings := DefaultSettings()
	if len(settings.Agents) == 0 {
		t.Fatal("expected default settings to include execution agents")
	}

	for _, agent := range settings.Agents {
		sidecar := agent.APISIXSidecar
		if sidecar.Image == "" {
			t.Fatalf("expected %s to include an APISIX sidecar image", agent.AgentID)
		}
		if sidecar.ConfigMountPath == "" {
			t.Fatalf("expected %s to include an APISIX config mount path", agent.AgentID)
		}
		if sidecar.ListenPort != 9080 {
			t.Fatalf("expected %s to use listen port 9080, got %d", agent.AgentID, sidecar.ListenPort)
		}
		if sidecar.AdminPort != 9180 {
			t.Fatalf("expected %s to use admin port 9180, got %d", agent.AgentID, sidecar.AdminPort)
		}
		if len(sidecar.Capabilities) == 0 {
			t.Fatalf("expected %s to advertise APISIX capabilities", agent.AgentID)
		}
	}
}

func TestNormalizeBackfillsAPISIXSidecarDefaults(t *testing.T) {
	settings := &PlatformSettings{
		Agents: []ExecutionAgent{
			{
				AgentID: "agent-1",
				Name:    "Agent 1",
				Type:    "local",
			},
		},
		Registries: []OCIRegistry{
			{
				RegistryID:  "registry-1",
				Name:        "Registry",
				Provider:    "Generic OCI",
				RegistryURL: "http://registry.internal",
			},
		},
	}

	normalize(settings)

	sidecar := settings.Agents[0].APISIXSidecar
	if sidecar.Image == "" {
		t.Fatal("expected normalize to backfill APISIX image")
	}
	if sidecar.ConfigMountPath == "" {
		t.Fatal("expected normalize to backfill APISIX config path")
	}
	if sidecar.ListenPort == 0 || sidecar.AdminPort == 0 {
		t.Fatalf("expected normalize to backfill APISIX ports, got listen=%d admin=%d", sidecar.ListenPort, sidecar.AdminPort)
	}
	if len(sidecar.Capabilities) == 0 {
		t.Fatal("expected normalize to backfill APISIX capabilities")
	}
}
