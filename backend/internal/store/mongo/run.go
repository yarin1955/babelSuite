package mongo

import (
	"context"
	"time"

	"github.com/babelsuite/babelsuite/internal/domain"
	"github.com/babelsuite/babelsuite/internal/store"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// ── runs ──────────────────────────────────────────────────────────────────────

func (s *Store) CreateRun(ctx context.Context, r *domain.Run) error {
	_, err := s.runs.InsertOne(ctx, r)
	return wrap(err)
}

func (s *Store) ListRuns(ctx context.Context, orgID string, page, pageSize int) ([]*domain.Run, int64, error) {
	filter := bson.M{"org_id": orgID}
	total, err := s.runs.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, wrap(err)
	}
	skip := int64((page - 1) * pageSize)
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetSkip(skip).SetLimit(int64(pageSize))
	cur, err := s.runs.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, wrap(err)
	}
	defer cur.Close(ctx)
	var list []*domain.Run
	if err := cur.All(ctx, &list); err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func (s *Store) GetRun(ctx context.Context, id string) (*domain.Run, error) {
	var r domain.Run
	return &r, wrap(s.runs.FindOne(ctx, bson.M{"run_id": id}).Decode(&r))
}

func (s *Store) UpdateRun(ctx context.Context, r *domain.Run) error {
	_, err := s.runs.ReplaceOne(ctx, bson.M{"run_id": r.RunID}, r)
	return wrap(err)
}

func (s *Store) NextPendingRun(ctx context.Context, orgID string) (*domain.Run, error) {
	filter := bson.M{"org_id": orgID, "status": domain.RunPending}
	update := bson.M{"$set": bson.M{"status": domain.RunRunning, "started_at": time.Now().UTC()}}
	opts := options.FindOneAndUpdate().SetSort(bson.D{{Key: "created_at", Value: 1}}).SetReturnDocument(options.After)
	var r domain.Run
	err := s.runs.FindOneAndUpdate(ctx, filter, update, opts).Decode(&r)
	if err != nil {
		return nil, wrap(err)
	}
	return &r, nil
}

// ── steps ─────────────────────────────────────────────────────────────────────

func (s *Store) CreateStep(ctx context.Context, step *domain.Step) error {
	_, err := s.steps.InsertOne(ctx, step)
	return wrap(err)
}

func (s *Store) ListSteps(ctx context.Context, runID string) ([]*domain.Step, error) {
	cur, err := s.steps.Find(ctx, bson.M{"run_id": runID}, options.Find().SetSort(bson.D{{Key: "position", Value: 1}}))
	if err != nil {
		return nil, wrap(err)
	}
	defer cur.Close(ctx)
	var list []*domain.Step
	if err := cur.All(ctx, &list); err != nil {
		return nil, err
	}
	return list, nil
}

func (s *Store) UpdateStep(ctx context.Context, step *domain.Step) error {
	_, err := s.steps.ReplaceOne(ctx, bson.M{"step_id": step.StepID}, step)
	return wrap(err)
}

// ── logs ──────────────────────────────────────────────────────────────────────

func (s *Store) AppendLogs(ctx context.Context, entries []*domain.LogEntry) error {
	if len(entries) == 0 {
		return nil
	}
	docs := make([]any, len(entries))
	for i, e := range entries {
		docs[i] = e
	}
	_, err := s.logs.InsertMany(ctx, docs)
	return wrap(err)
}

func (s *Store) GetLogs(ctx context.Context, stepID string) ([]*domain.LogEntry, error) {
	opts := options.Find().SetSort(bson.D{{Key: "line", Value: 1}})
	cur, err := s.logs.Find(ctx, bson.M{"step_id": stepID}, opts)
	if err != nil {
		if err.Error() == store.ErrNotFound.Error() {
			return nil, nil
		}
		return nil, wrap(err)
	}
	defer cur.Close(ctx)
	var list []*domain.LogEntry
	if err := cur.All(ctx, &list); err != nil {
		return nil, err
	}
	return list, nil
}
