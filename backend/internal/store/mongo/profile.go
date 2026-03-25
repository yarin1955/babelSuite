package mongo

import (
	"context"

	"github.com/babelsuite/babelsuite/internal/domain"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func (s *Store) CreateProfile(ctx context.Context, p *domain.Profile) error {
	_, err := s.profiles.InsertOne(ctx, p)
	return wrap(err)
}

func (s *Store) ListProfiles(ctx context.Context, orgID string) ([]*domain.Profile, error) {
	cur, err := s.profiles.Find(ctx, bson.M{"org_id": orgID}, options.Find().SetSort(bson.D{{Key: "updated_at", Value: -1}}))
	if err != nil {
		return nil, wrap(err)
	}
	defer cur.Close(ctx)
	var list []*domain.Profile
	if err := cur.All(ctx, &list); err != nil {
		return nil, err
	}
	return list, nil
}

func (s *Store) GetProfile(ctx context.Context, id string) (*domain.Profile, error) {
	var p domain.Profile
	return &p, wrap(s.profiles.FindOne(ctx, bson.M{"profile_id": id}).Decode(&p))
}

func (s *Store) UpdateProfile(ctx context.Context, p *domain.Profile) error {
	_, err := s.profiles.ReplaceOne(ctx, bson.M{"profile_id": p.ProfileID}, p)
	return wrap(err)
}

func (s *Store) DeleteProfile(ctx context.Context, id string) error {
	_, err := s.profiles.DeleteOne(ctx, bson.M{"profile_id": id})
	return wrap(err)
}
