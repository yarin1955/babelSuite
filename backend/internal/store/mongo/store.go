package mongo

import (
	"context"
	"errors"
	"time"

	"github.com/babelsuite/babelsuite/internal/domain"
	"github.com/babelsuite/babelsuite/internal/store"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type Store struct {
	client        *mongo.Client
	users         *mongo.Collection
	orgs          *mongo.Collection
	registries    *mongo.Collection
	packages      *mongo.Collection
	oidcProviders *mongo.Collection
	agents        *mongo.Collection
	runs          *mongo.Collection
	steps         *mongo.Collection
	logs          *mongo.Collection
}

func New(uri, dbName string) (*Store, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx, nil); err != nil {
		return nil, err
	}

	db := client.Database(dbName)
	s := &Store{
		client:        client,
		users:         db.Collection("users"),
		orgs:          db.Collection("orgs"),
		registries:    db.Collection("registries"),
		packages:      db.Collection("catalog_packages"),
		oidcProviders: db.Collection("oidc_providers"),
		agents:        db.Collection("agents"),
		runs:          db.Collection("runs"),
		steps:         db.Collection("steps"),
		logs:          db.Collection("run_logs"),
	}

	uniq := options.Index().SetUnique(true)
	s.users.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "username", Value: 1}}, Options: uniq})
	s.users.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "email", Value: 1}}, Options: uniq})
	s.orgs.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "slug", Value: 1}}, Options: uniq})
	s.packages.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "org_id", Value: 1}, {Key: "image_ref", Value: 1}}, Options: uniq})
	s.agents.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "token", Value: 1}}, Options: uniq})
	s.steps.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "run_id", Value: 1}}})
	s.logs.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "step_id", Value: 1}, {Key: "line", Value: 1}}})
	return s, nil
}

func (s *Store) Close(ctx context.Context) error { return s.client.Disconnect(ctx) }

func wrap(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, mongo.ErrNoDocuments) {
		return store.ErrNotFound
	}
	if mongo.IsDuplicateKeyError(err) {
		return store.ErrDuplicate
	}
	return err
}

func (s *Store) CreateOrg(ctx context.Context, o *domain.Org) error {
	_, err := s.orgs.InsertOne(ctx, o)
	return wrap(err)
}

func (s *Store) GetOrgBySlug(ctx context.Context, slug string) (*domain.Org, error) {
	var o domain.Org
	return &o, wrap(s.orgs.FindOne(ctx, bson.M{"slug": slug}).Decode(&o))
}

func (s *Store) GetOrgByID(ctx context.Context, id string) (*domain.Org, error) {
	var o domain.Org
	return &o, wrap(s.orgs.FindOne(ctx, bson.M{"org_id": id}).Decode(&o))
}

func (s *Store) CreateUser(ctx context.Context, u *domain.User) error {
	_, err := s.users.InsertOne(ctx, u)
	return wrap(err)
}

func (s *Store) GetUserByID(ctx context.Context, id string) (*domain.User, error) {
	var u domain.User
	return &u, wrap(s.users.FindOne(ctx, bson.M{"user_id": id}).Decode(&u))
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (*domain.User, error) {
	var u domain.User
	return &u, wrap(s.users.FindOne(ctx, bson.M{"username": username}).Decode(&u))
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	var u domain.User
	return &u, wrap(s.users.FindOne(ctx, bson.M{"email": email}).Decode(&u))
}
