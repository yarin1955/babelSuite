package postgres

import (
	"context"
	"time"

	"github.com/babelsuite/babelsuite/internal/domain"
	"github.com/jackc/pgx/v5"
)

type pgxBatch = pgx.Batch

// ── runs ──────────────────────────────────────────────────────────────────────

func (s *Store) CreateRun(ctx context.Context, r *domain.Run) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO runs(run_id,org_id,package_id,image_ref,profile,agent_id,status,started_at,finished_at,created_at)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		r.RunID, r.OrgID, r.PackageID, r.ImageRef, r.Profile, r.AgentID, r.Status,
		r.StartedAt, r.FinishedAt, r.CreatedAt)
	return wrap(err)
}

func (s *Store) ListRuns(ctx context.Context, orgID string, page, pageSize int) ([]*domain.Run, int64, error) {
	var total int64
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM runs WHERE org_id=$1`, orgID).Scan(&total)
	if err != nil {
		return nil, 0, wrap(err)
	}
	offset := (page - 1) * pageSize
	rows, err := s.pool.Query(ctx,
		`SELECT run_id,org_id,package_id,image_ref,profile,agent_id,status,started_at,finished_at,created_at
		 FROM runs WHERE org_id=$1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		orgID, pageSize, offset)
	if err != nil {
		return nil, 0, wrap(err)
	}
	defer rows.Close()
	var list []*domain.Run
	for rows.Next() {
		var r domain.Run
		if err := rows.Scan(&r.RunID, &r.OrgID, &r.PackageID, &r.ImageRef, &r.Profile, &r.AgentID,
			&r.Status, &r.StartedAt, &r.FinishedAt, &r.CreatedAt); err != nil {
			return nil, 0, err
		}
		list = append(list, &r)
	}
	return list, total, nil
}

func (s *Store) GetRun(ctx context.Context, id string) (*domain.Run, error) {
	var r domain.Run
	err := s.pool.QueryRow(ctx,
		`SELECT run_id,org_id,package_id,image_ref,profile,agent_id,status,started_at,finished_at,created_at
		 FROM runs WHERE run_id=$1`, id).
		Scan(&r.RunID, &r.OrgID, &r.PackageID, &r.ImageRef, &r.Profile, &r.AgentID,
			&r.Status, &r.StartedAt, &r.FinishedAt, &r.CreatedAt)
	return &r, wrap(err)
}

func (s *Store) UpdateRun(ctx context.Context, r *domain.Run) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE runs SET agent_id=$2,status=$3,started_at=$4,finished_at=$5 WHERE run_id=$1`,
		r.RunID, r.AgentID, r.Status, r.StartedAt, r.FinishedAt)
	return wrap(err)
}

func (s *Store) NextPendingRun(ctx context.Context, orgID, agentID string) (*domain.Run, error) {
	now := time.Now().UTC()
	var r domain.Run
	err := s.pool.QueryRow(ctx,
		`UPDATE runs SET agent_id=$3, status='running', started_at=$2
		 WHERE run_id = (
		   SELECT run_id FROM runs WHERE org_id=$1 AND status='pending'
		   ORDER BY created_at ASC LIMIT 1
		   FOR UPDATE SKIP LOCKED
		 )
		 RETURNING run_id,org_id,package_id,image_ref,profile,agent_id,status,started_at,finished_at,created_at`,
		orgID, now, agentID).
		Scan(&r.RunID, &r.OrgID, &r.PackageID, &r.ImageRef, &r.Profile, &r.AgentID,
			&r.Status, &r.StartedAt, &r.FinishedAt, &r.CreatedAt)
	return &r, wrap(err)
}

func (s *Store) CountActiveRunsByAgent(ctx context.Context, agentID string) (int64, error) {
	var count int64
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM runs WHERE agent_id=$1 AND status IN ('pending', 'running')`,
		agentID,
	).Scan(&count)
	return count, wrap(err)
}

// ── steps ─────────────────────────────────────────────────────────────────────

func (s *Store) CreateStep(ctx context.Context, step *domain.Step) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO steps(step_id,run_id,name,position,type,status,exit_code,error,started_at,finished_at)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		step.StepID, step.RunID, step.Name, step.Position, step.Type, step.Status, step.ExitCode,
		step.Error, step.StartedAt, step.FinishedAt)
	return wrap(err)
}

func (s *Store) ListSteps(ctx context.Context, runID string) ([]*domain.Step, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT step_id,run_id,name,position,type,status,exit_code,error,started_at,finished_at
		 FROM steps WHERE run_id=$1 ORDER BY position ASC`, runID)
	if err != nil {
		return nil, wrap(err)
	}
	defer rows.Close()
	var list []*domain.Step
	for rows.Next() {
		var step domain.Step
		if err := rows.Scan(&step.StepID, &step.RunID, &step.Name, &step.Position, &step.Type, &step.Status,
			&step.ExitCode, &step.Error, &step.StartedAt, &step.FinishedAt); err != nil {
			return nil, err
		}
		list = append(list, &step)
	}
	return list, nil
}

func (s *Store) UpdateStep(ctx context.Context, step *domain.Step) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE steps SET name=$2,status=$3,exit_code=$4,error=$5,started_at=$6,finished_at=$7 WHERE step_id=$1`,
		step.StepID, step.Name, step.Status, step.ExitCode, step.Error, step.StartedAt, step.FinishedAt)
	return wrap(err)
}

// ── logs ──────────────────────────────────────────────────────────────────────

func (s *Store) AppendLogs(ctx context.Context, entries []*domain.LogEntry) error {
	if len(entries) == 0 {
		return nil
	}
	batch := &pgxBatch{}
	for _, e := range entries {
		batch.Queue(
			`INSERT INTO run_logs(log_id,run_id,step_id,line,data,time,type,trace_id,span_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT DO NOTHING`,
			e.LogID, e.RunID, e.StepID, e.Line, e.Data, e.Time, int(e.Type), e.TraceID, e.SpanID)
	}
	return s.pool.SendBatch(ctx, batch).Close()
}

func (s *Store) GetLogs(ctx context.Context, stepID string) ([]*domain.LogEntry, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT log_id,run_id,step_id,line,data,time,type,trace_id,span_id FROM run_logs WHERE step_id=$1 ORDER BY line ASC`,
		stepID)
	if err != nil {
		return nil, wrap(err)
	}
	defer rows.Close()
	var list []*domain.LogEntry
	for rows.Next() {
		var e domain.LogEntry
		var t int
		if err := rows.Scan(&e.LogID, &e.RunID, &e.StepID, &e.Line, &e.Data, &e.Time, &t, &e.TraceID, &e.SpanID); err != nil {
			return nil, err
		}
		e.Type = domain.LogEntryType(t)
		list = append(list, &e)
	}
	return list, nil
}
