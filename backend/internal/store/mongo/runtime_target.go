package mongo

import (
	"context"

	"github.com/babelsuite/babelsuite/internal/domain"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func (s *Store) CreateRuntimeTarget(ctx context.Context, t *domain.RuntimeTarget) error {
	_, err := s.runtimeTargets.InsertOne(ctx, t)
	return wrap(err)
}

func (s *Store) ListRuntimeTargets(ctx context.Context, orgID string) ([]*domain.RuntimeTarget, error) {
	cur, err := s.runtimeTargets.Find(ctx, bson.M{"org_id": orgID}, options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
	if err != nil {
		return nil, wrap(err)
	}
	defer cur.Close(ctx)
	var list []*domain.RuntimeTarget
	if err := cur.All(ctx, &list); err != nil {
		return nil, err
	}
	return list, nil
}

func (s *Store) GetRuntimeTarget(ctx context.Context, id string) (*domain.RuntimeTarget, error) {
	var target domain.RuntimeTarget
	return &target, wrap(s.runtimeTargets.FindOne(ctx, bson.M{"runtime_target_id": id}).Decode(&target))
}

func (s *Store) UpdateRuntimeTarget(ctx context.Context, t *domain.RuntimeTarget) error {
	_, err := s.runtimeTargets.ReplaceOne(ctx, bson.M{"runtime_target_id": t.RuntimeTargetID}, t)
	return wrap(err)
}

func (s *Store) DeleteRuntimeTarget(ctx context.Context, id string) error {
	_, err := s.runtimeTargets.DeleteOne(ctx, bson.M{"runtime_target_id": id})
	return wrap(err)
}
