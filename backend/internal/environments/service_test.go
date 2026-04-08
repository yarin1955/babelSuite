package environments

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSnapshotBuildsZombieSandboxInventory(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 27, 18, 0, 0, 0, time.UTC)
	runner := &fakeRunner{
		outputs: map[string]string{
			cmd("docker", "ps", "-aq", "--filter", "label=babelsuite.managed=true"): "container-a\ncontainer-b\n",
			cmd("docker", "inspect", "container-a", "container-b"): `[
  {
    "Id": "container-a",
    "Name": "/payment-gateway",
    "Config": {
      "Image": "ghcr.io/example/payment-gateway:latest",
      "Labels": {
        "babelsuite.managed": "true",
        "babelsuite.run_id": "run-2049",
        "babelsuite.suite": "Payment Suite",
        "babelsuite.owner": "Platform QA",
        "babelsuite.profile": "staging.yaml",
        "babelsuite.orchestrator_pid": "5012",
        "babelsuite.last_heartbeat": "2026-03-27T17:55:00Z"
      }
    },
    "State": {
      "Status": "running",
      "StartedAt": "2026-03-27T17:40:00Z",
      "ExitCode": 0
    },
    "NetworkSettings": {
      "Ports": {
        "8080/tcp": [
          { "HostIp": "0.0.0.0", "HostPort": "18080" }
        ]
      }
    }
  },
  {
    "Id": "container-b",
    "Name": "/fraud-worker",
    "Config": {
      "Image": "ghcr.io/example/fraud-worker:latest",
      "Labels": {
        "babelsuite.managed": "true",
        "babelsuite.run_id": "run-2049",
        "babelsuite.suite": "Payment Suite",
        "babelsuite.owner": "Platform QA",
        "babelsuite.profile": "staging.yaml",
        "babelsuite.orchestrator_pid": "5012",
        "babelsuite.last_heartbeat": "2026-03-27T17:55:00Z"
      }
    },
    "State": {
      "Status": "exited",
      "StartedAt": "2026-03-27T17:41:00Z",
      "ExitCode": 1
    },
    "NetworkSettings": {
      "Ports": {}
    }
  }
]`,
			cmd("docker", "network", "ls", "-q", "--filter", "label=babelsuite.managed=true"): "network-a\n",
			cmd("docker", "network", "inspect", "network-a"): `[
  {
    "Id": "network-a",
    "Name": "payment-suite_default",
    "Driver": "bridge",
    "Scope": "local",
    "Labels": {
      "babelsuite.managed": "true",
      "babelsuite.run_id": "run-2049",
      "babelsuite.suite": "Payment Suite"
    }
  }
]`,
			cmd("docker", "volume", "ls", "-q", "--filter", "label=babelsuite.managed=true"): "volume-a\n",
			cmd("docker", "volume", "inspect", "volume-a"): `[
  {
    "Name": "payment-suite_db",
    "Driver": "local",
    "Mountpoint": "/var/lib/docker/volumes/payment-suite_db/_data",
    "Labels": {
      "babelsuite.managed": "true",
      "babelsuite.run_id": "run-2049",
      "babelsuite.suite": "Payment Suite"
    }
  }
]`,
			cmd("docker", "stats", "--no-stream", "--format", "{{json .}}", "container-a"): `{"ID":"container-a","Name":"payment-gateway","CPUPerc":"22.40%","MemUsage":"128MiB / 512MiB","MemPerc":"25.00%"}`,
		},
	}

	service := newServiceForTest(runner, fakeChecker{alive: map[int]bool{5012: false}}, func() time.Time { return now })
	inventory, err := service.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	if !inventory.DockerAvailable {
		t.Fatal("expected docker inventory to be available")
	}
	if len(inventory.Sandboxes) != 1 {
		t.Fatalf("expected 1 sandbox, got %d", len(inventory.Sandboxes))
	}

	sandbox := inventory.Sandboxes[0]
	if sandbox.Status != "Zombie" {
		t.Fatalf("expected zombie status, got %q", sandbox.Status)
	}
	if !sandbox.IsZombie {
		t.Fatal("expected sandbox to be marked zombie")
	}
	if sandbox.ResourceUsage.MemoryBytes <= 0 {
		t.Fatal("expected aggregated memory usage")
	}
	if len(sandbox.Networks) != 1 || len(sandbox.Volumes) != 1 {
		t.Fatalf("unexpected resource map: %+v", sandbox)
	}
	if inventory.Summary.ZombieSandboxes != 1 {
		t.Fatalf("expected zombie summary count, got %+v", inventory.Summary)
	}
}

func TestReapSandboxRemovesManagedResources(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{
		outputs: map[string]string{
			cmd("docker", "ps", "-aq", "--filter", "label=babelsuite.managed=true"): "container-a\n",
			cmd("docker", "inspect", "container-a"): `[
  {
    "Id": "container-a",
    "Name": "/payment-gateway",
    "Config": {
      "Image": "ghcr.io/example/payment-gateway:latest",
      "Labels": {
        "babelsuite.managed": "true",
        "babelsuite.run_id": "run-2049",
        "babelsuite.suite": "Payment Suite"
      }
    },
    "State": {
      "Status": "running",
      "StartedAt": "2026-03-27T17:40:00Z",
      "ExitCode": 0
    },
    "NetworkSettings": {
      "Ports": {}
    }
  }
]`,
			cmd("docker", "network", "ls", "-q", "--filter", "label=babelsuite.managed=true"): "network-a\n",
			cmd("docker", "network", "inspect", "network-a"): `[
  {
    "Id": "network-a",
    "Name": "payment-suite_default",
    "Driver": "bridge",
    "Scope": "local",
    "Labels": {
      "babelsuite.managed": "true",
      "babelsuite.run_id": "run-2049"
    }
  }
]`,
			cmd("docker", "volume", "ls", "-q", "--filter", "label=babelsuite.managed=true"): "volume-a\n",
			cmd("docker", "volume", "inspect", "volume-a"): `[
  {
    "Name": "payment-suite_db",
    "Driver": "local",
    "Mountpoint": "/var/lib/docker/volumes/payment-suite_db/_data",
    "Labels": {
      "babelsuite.managed": "true",
      "babelsuite.run_id": "run-2049"
    }
  }
]`,
			cmd("docker", "rm", "-f", "container-a"):                "",
			cmd("docker", "network", "rm", "network-a"):             "",
			cmd("docker", "volume", "rm", "-f", "payment-suite_db"): "",
		},
	}

	service := newServiceForTest(runner, fakeChecker{}, func() time.Time { return time.Now().UTC() })
	result, err := service.ReapSandbox(context.Background(), "run-2049")
	if err != nil {
		t.Fatalf("reap sandbox: %v", err)
	}

	if result.RemovedContainers != 1 || result.RemovedNetworks != 1 || result.RemovedVolumes != 1 {
		t.Fatalf("unexpected reap result: %+v", result)
	}

	recorded := strings.Join(runner.calls, "\n")
	for _, expected := range []string{
		cmd("docker", "rm", "-f", "container-a"),
		cmd("docker", "network", "rm", "network-a"),
		cmd("docker", "volume", "rm", "-f", "payment-suite_db"),
	} {
		if !strings.Contains(recorded, expected) {
			t.Fatalf("expected command %q to be executed, got:\n%s", expected, recorded)
		}
	}
}

func TestSubscribeEventsReplaysAndStreamsSnapshotChanges(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)
	runner := &fakeRunner{
		outputs: map[string]string{
			cmd("docker", "ps", "-aq", "--filter", "label=babelsuite.managed=true"): "container-a\n",
			cmd("docker", "inspect", "container-a"): `[
  {
    "Id": "container-a",
    "Name": "/payment-gateway",
    "Config": {
      "Image": "ghcr.io/example/payment-gateway:latest",
      "Labels": {
        "babelsuite.managed": "true",
        "babelsuite.run_id": "run-2049",
        "babelsuite.suite": "Payment Suite"
      }
    },
    "State": {
      "Status": "running",
      "StartedAt": "2026-03-28T09:58:00Z",
      "ExitCode": 0
    },
    "NetworkSettings": {
      "Ports": {}
    }
  }
]`,
			cmd("docker", "network", "ls", "-q", "--filter", "label=babelsuite.managed=true"): "",
			cmd("docker", "volume", "ls", "-q", "--filter", "label=babelsuite.managed=true"):  "",
			cmd("docker", "stats", "--no-stream", "--format", "{{json .}}", "container-a"):    "",
		},
	}

	service := newServiceForTest(runner, fakeChecker{alive: map[int]bool{}}, func() time.Time { return now })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream, err := service.SubscribeEvents(ctx, 0)
	if err != nil {
		t.Fatalf("subscribe events: %v", err)
	}

	select {
	case event := <-stream:
		if event.ID != 1 {
			t.Fatalf("expected replay event id 1, got %d", event.ID)
		}
		if len(event.Snapshot.Sandboxes) != 1 {
			t.Fatalf("expected 1 sandbox in replay, got %d", len(event.Snapshot.Sandboxes))
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for replay snapshot")
	}

	runner.outputs[cmd("docker", "ps", "-aq", "--filter", "label=babelsuite.managed=true")] = ""
	delete(runner.outputs, cmd("docker", "inspect", "container-a"))

	if err := service.publishLatestSnapshot(context.Background(), "test-update", false); err != nil {
		t.Fatalf("publish updated snapshot: %v", err)
	}

	select {
	case event := <-stream:
		if event.ID != 2 {
			t.Fatalf("expected live event id 2, got %d", event.ID)
		}
		if len(event.Snapshot.Sandboxes) != 0 {
			t.Fatalf("expected empty sandbox inventory after update, got %d", len(event.Snapshot.Sandboxes))
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for live snapshot")
	}
}

type fakeRunner struct {
	outputs map[string]string
	calls   []string
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	key := cmd(name, args...)
	f.calls = append(f.calls, key)
	output, ok := f.outputs[key]
	if !ok {
		return nil, errors.New("unexpected command: " + key)
	}
	return []byte(output), nil
}

type fakeChecker struct {
	alive map[int]bool
}

func (f fakeChecker) Alive(_ context.Context, pid int) bool {
	if f.alive == nil {
		return false
	}
	return f.alive[pid]
}

func cmd(name string, args ...string) string {
	return strings.TrimSpace(name + " " + strings.Join(args, " "))
}
