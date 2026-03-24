package demo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/babelsuite/babelsuite/internal/auth"
	"github.com/babelsuite/babelsuite/internal/domain"
	"github.com/babelsuite/babelsuite/internal/engine"
	"github.com/babelsuite/babelsuite/internal/runs"
	"github.com/babelsuite/babelsuite/internal/store"
	"github.com/google/uuid"
)

// Handler runs the fleet-integration-suite demo using real Docker containers.
// Each step is a genuine container: postgres:15-alpine produces real startup
// logs, psql runs real DDL, and python:3.11-slim runs real pytest assertions.
type Handler struct {
	store  store.Store
	jwt    *auth.JWTService
	docker *engine.Docker
	logPub *runs.LogPubSub
	runPub *runs.RunPubSub
}

func NewHandler(s store.Store, jwt *auth.JWTService, runsHandler *runs.Handler) *Handler {
	docker, err := engine.NewDocker()
	if err != nil {
		log.Printf("demo: docker unavailable (%v) — demo endpoint disabled", err)
		return &Handler{store: s, jwt: jwt, logPub: runsHandler.LogPubSub(), runPub: runsHandler.RunPubSub()}
	}
	return &Handler{
		store:  s,
		jwt:    jwt,
		docker: docker,
		logPub: runsHandler.LogPubSub(),
		runPub: runsHandler.RunPubSub(),
	}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/demo/run", auth.Middleware(h.jwt)(http.HandlerFunc(h.startRun)).ServeHTTP)
}

func (h *Handler) startRun(w http.ResponseWriter, r *http.Request) {
	if h.docker == nil {
		writeErr(w, http.StatusServiceUnavailable, "Docker unavailable — demo disabled")
		return
	}
	claims := auth.ClaimsFrom(r)

	run := &domain.Run{
		RunID:     uuid.NewString(),
		OrgID:     claims.OrgID,
		PackageID: "ghcr.io/demo-org/fleet-suite",
		ImageRef:  "ghcr.io/demo-org/fleet-suite:v1.4.2",
		Status:    domain.RunPending,
		CreatedAt: time.Now().UTC(),
	}
	if err := h.store.CreateRun(r.Context(), run); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	go h.execute(run)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(run)
}

// execute builds the workflow config and runs it via the Runtime executor.
// The workflow mirrors run_fleet_suite from explain.md:
//
//	Stage 1 — infrastructure:  postgres:15-alpine (detached service)
//	Stage 2 — migrations:      psql applies DDL against pg-primary
//	Stage 3 — tests:           python:3.11-slim runs pytest against live postgres
func (h *Handler) execute(run *domain.Run) {
	ctx := context.Background()
	taskUUID := run.RunID

	// pg-primary is the hostname other containers use to reach postgres.
	// It is set as a network alias when the container joins the workflow network.
	pgName := "pg-primary"

	migrationScript := fmt.Sprintf(`set -e
until pg_isready -h %s -U admin -q; do
    echo "waiting for PostgreSQL..." && sleep 1
done
echo "PostgreSQL ready."
PGPASSWORD=pw psql -h %s -U admin -d fleet_db -v ON_ERROR_STOP=1 <<SQL
CREATE TABLE IF NOT EXISTS devices (
    id         SERIAL PRIMARY KEY,
    name       TEXT   NOT NULL UNIQUE,
    status     TEXT   NOT NULL DEFAULT 'active',
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
INSERT INTO devices (name, status) VALUES
    ('drone-alpha-001',    'active'),
    ('drone-beta-002',     'active'),
    ('drone-gamma-003',    'active'),
    ('ground-station-001', 'online')
ON CONFLICT DO NOTHING;
SELECT 'migration 001_create_devices: OK' AS result;
SELECT '  devices registered: ' || COUNT(*) AS result FROM devices;
SQL
echo "All migrations applied."`, pgName, pgName)

	testScript := fmt.Sprintf(`set -e
pip install psycopg2-binary pytest -q
cat > /tmp/test_fleet_db.py << 'PYEOF'
import psycopg2
import pytest

DSN = "postgresql://admin:pw@%s:5432/fleet_db"

@pytest.fixture(scope="module")
def db():
    conn = psycopg2.connect(DSN)
    yield conn
    conn.close()

def test_connection(db):
    assert db.closed == 0

def test_devices_table_exists(db):
    cur = db.cursor()
    cur.execute(
        "SELECT 1 FROM information_schema.tables "
        "WHERE table_schema='public' AND table_name='devices'"
    )
    assert cur.fetchone(), "devices table not found"

def test_fleet_seeded(db):
    cur = db.cursor()
    cur.execute("SELECT COUNT(*) FROM devices")
    assert cur.fetchone()[0] >= 4

def test_drone_alpha_active(db):
    cur = db.cursor()
    cur.execute("SELECT status FROM devices WHERE name='drone-alpha-001'")
    row = cur.fetchone()
    assert row and row[0] == "active"

def test_ground_station_online(db):
    cur = db.cursor()
    cur.execute("SELECT status FROM devices WHERE name='ground-station-001'")
    row = cur.fetchone()
    assert row and row[0] == "online"
PYEOF
pytest /tmp/test_fleet_db.py -v --tb=short`, pgName)

	conf := &engine.WorkflowConfig{
		Network: "babel-demo-" + taskUUID[:8],
		Stages: []*engine.WorkflowStage{
			{
				// Stage 1 — infrastructure: postgres runs as a detached service.
				// The Runtime starts it and leaves it running for subsequent stages.
				Steps: []*engine.WorkflowStep{
					{
						UUID:     uuid.NewString(),
						Name:     pgName,
						Image:    "postgres:15-alpine",
						Pull:     true,
						Detached: true,
						Env: map[string]string{
							"POSTGRES_USER":     "admin",
							"POSTGRES_PASSWORD": "pw",
							"POSTGRES_DB":       "fleet_db",
						},
					},
				},
			},
			{
				// Stage 2 — migrations: psql client connects to pg-primary.
				Steps: []*engine.WorkflowStep{
					{
						UUID:  uuid.NewString(),
						Name:  "migrations",
						Image: "postgres:15-alpine",
						Cmd:   []string{"sh", "-c", migrationScript},
					},
				},
			},
			{
				// Stage 3 — integration tests: pytest against live postgres.
				Steps: []*engine.WorkflowStep{
					{
						UUID:  uuid.NewString(),
						Name:  "pytest-fleet",
						Image: "python:3.11-slim",
						Cmd:   []string{"sh", "-c", testScript},
					},
				},
			},
		},
	}

	// Mark run as running before kicking off the Runtime.
	now := time.Now().UTC()
	run.Status = domain.RunRunning
	run.StartedAt = &now
	_ = h.store.UpdateRun(ctx, run)
	h.runPub.Publish(run)

	// stepSteps tracks the domain.Step created for each WorkflowStep so the
	// logger goroutine can publish logs under the correct StepID.
	stepMap := map[string]*domain.Step{} // WorkflowStep.UUID → domain.Step

	// Allocate domain.Step records upfront so SSE clients can subscribe before
	// the containers start.
	for _, stage := range conf.Stages {
		for _, ws := range stage.Steps {
			stepNow := time.Now().UTC()
			ds := &domain.Step{
				StepID:    uuid.NewString(),
				RunID:     run.RunID,
				Name:      ws.Name,
				Status:    domain.RunPending,
				StartedAt: &stepNow,
			}
			_ = h.store.CreateStep(ctx, ds)
			stepMap[ws.UUID] = ds
		}
	}

	// logger is called by the Runtime for every step. It reads lines from the
	// container's stdout+stderr and publishes them to the LogPubSub.
	logger := func(ws *engine.WorkflowStep, rc io.ReadCloser) error {
		ds, ok := stepMap[ws.UUID]
		if !ok {
			return nil
		}
		// Mark step running.
		stepNow := time.Now().UTC()
		ds.Status = domain.RunRunning
		ds.StartedAt = &stepNow
		_ = h.store.UpdateStep(ctx, ds)

		lineNum := 0
		lw := engine.NewLineWriter(func(line []byte) {
			// Trim the trailing newline for storage; the UI re-adds it.
			data := string(line)
			if len(data) > 0 && data[len(data)-1] == '\n' {
				data = data[:len(data)-1]
			}
			entry := &domain.LogEntry{
				LogID:  uuid.NewString(),
				RunID:  run.RunID,
				StepID: ds.StepID,
				Line:   lineNum,
				Data:   data,
				Time:   time.Now().UnixMilli(),
				Type:   domain.LogStdout,
			}
			_ = h.store.AppendLogs(ctx, []*domain.LogEntry{entry})
			h.logPub.Publish(ds.StepID, []*domain.LogEntry{entry})
			lineNum++
		})
		err := lw.WriteTo(rc)

		if !ws.Detached {
			// Signal EOF to SSE clients and mark step succeeded.
			// Failure is determined by the Runtime from the exit code.
			h.logPub.Publish(ds.StepID, nil)
			stepEnd := time.Now().UTC()
			ds.Status = domain.RunSuccess
			ds.FinishedAt = &stepEnd
			_ = h.store.UpdateStep(ctx, ds)
		}
		return err
	}

	rt := engine.NewRuntime(h.docker, conf, taskUUID, logger)
	err := rt.Run(ctx)
	if err != nil {
		log.Printf("demo run %s failed: %v", run.RunID, err)
	}

	end := time.Now().UTC()
	if err != nil {
		run.Status = domain.RunFailure
	} else {
		run.Status = domain.RunSuccess
	}
	run.FinishedAt = &end
	_ = h.store.UpdateRun(ctx, run)
	h.runPub.Publish(run)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
