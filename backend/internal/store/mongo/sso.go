package mongo

import (
	"context"
	"time"

	"github.com/babelsuite/babelsuite/internal/domain"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func (s *Store) CreateOIDCProvider(ctx context.Context, p *domain.OIDCProvider) error {
	_, err := s.oidcProviders.InsertOne(ctx, p)
	return wrap(err)
}

func (s *Store) ListOIDCProviders(ctx context.Context) ([]*domain.OIDCProvider, error) {
	cur, err := s.oidcProviders.Find(ctx, bson.M{})
	if err != nil {
		return nil, wrap(err)
	}
	defer cur.Close(ctx)
	var list []*domain.OIDCProvider
	if err := cur.All(ctx, &list); err != nil {
		return nil, err
	}
	return list, nil
}

func (s *Store) GetOIDCProvider(ctx context.Context, id string) (*domain.OIDCProvider, error) {
	var p domain.OIDCProvider
	return &p, wrap(s.oidcProviders.FindOne(ctx, bson.M{"provider_id": id}).Decode(&p))
}

func (s *Store) UpdateOIDCProvider(ctx context.Context, p *domain.OIDCProvider) error {
	_, err := s.oidcProviders.ReplaceOne(ctx, bson.M{"provider_id": p.ProviderID}, p)
	return wrap(err)
}

func (s *Store) DeleteOIDCProvider(ctx context.Context, id string) error {
	_, err := s.oidcProviders.DeleteOne(ctx, bson.M{"provider_id": id})
	return wrap(err)
}

func (s *Store) UpsertUserByEmail(ctx context.Context, u *domain.User) (*domain.User, error) {
	filter := bson.M{"email": u.Email}
	update := bson.M{
		"$setOnInsert": bson.M{
			"user_id":    u.UserID,
			"org_id":     u.OrgID,
			"email":      u.Email,
			"pass_hash":  u.PassHash,
			"is_admin":   u.IsAdmin,
			"created_at": time.Now().UTC(),
		},
		"$set": bson.M{
			"username": u.Username,
			"name":     u.Name,
		},
	}
	opts := options.UpdateOne().SetUpsert(true)
	_, err := s.users.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return nil, wrap(err)
	}
	return s.GetUserByEmail(ctx, u.Email)
}
