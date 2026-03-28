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
	client           *mongo.Client
	users            *mongo.Collection
	workspaces       *mongo.Collection
	favoritePackages *mongo.Collection
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
	st := &Store{
		client:           client,
		users:            db.Collection("users"),
		workspaces:       db.Collection("workspaces"),
		favoritePackages: db.Collection("favorite_packages"),
	}

	unique := options.Index().SetUnique(true)
	_, _ = st.users.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "email", Value: 1}}, Options: unique})
	_, _ = st.users.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "username", Value: 1}}, Options: unique})
	_, _ = st.users.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "user_id", Value: 1}}, Options: unique})
	_, _ = st.workspaces.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "slug", Value: 1}}, Options: unique})
	_, _ = st.workspaces.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "workspace_id", Value: 1}}, Options: unique})
	_, _ = st.favoritePackages.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "user_id", Value: 1}, {Key: "package_id", Value: 1}},
		Options: unique,
	})

	return st, nil
}

func (s *Store) Close(ctx context.Context) error {
	return s.client.Disconnect(ctx)
}

func (s *Store) CreateWorkspace(ctx context.Context, workspace *domain.Workspace) error {
	_, err := s.workspaces.InsertOne(ctx, workspace)
	return wrap(err)
}

func (s *Store) GetWorkspaceByID(ctx context.Context, id string) (*domain.Workspace, error) {
	var workspace domain.Workspace
	err := s.workspaces.FindOne(ctx, bson.M{"workspace_id": id}).Decode(&workspace)
	return &workspace, wrap(err)
}

func (s *Store) GetWorkspaceBySlug(ctx context.Context, slug string) (*domain.Workspace, error) {
	var workspace domain.Workspace
	err := s.workspaces.FindOne(ctx, bson.M{"slug": slug}).Decode(&workspace)
	return &workspace, wrap(err)
}

func (s *Store) CreateUser(ctx context.Context, user *domain.User) error {
	_, err := s.users.InsertOne(ctx, user)
	return wrap(err)
}

func (s *Store) GetUserByID(ctx context.Context, id string) (*domain.User, error) {
	var user domain.User
	err := s.users.FindOne(ctx, bson.M{"user_id": id}).Decode(&user)
	return &user, wrap(err)
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	var user domain.User
	err := s.users.FindOne(ctx, bson.M{"email": email}).Decode(&user)
	return &user, wrap(err)
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (*domain.User, error) {
	var user domain.User
	err := s.users.FindOne(ctx, bson.M{"username": username}).Decode(&user)
	return &user, wrap(err)
}

func (s *Store) ListFavoritePackageIDs(ctx context.Context, userID string) ([]string, error) {
	cursor, err := s.favoritePackages.Find(ctx, bson.M{"user_id": userID})
	if err != nil {
		return nil, wrap(err)
	}
	defer cursor.Close(ctx)

	packageIDs := make([]string, 0)
	for cursor.Next(ctx) {
		var favorite domain.FavoritePackage
		if err := cursor.Decode(&favorite); err != nil {
			return nil, err
		}
		if favorite.PackageID != "" {
			packageIDs = append(packageIDs, favorite.PackageID)
		}
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}

	return packageIDs, nil
}

func (s *Store) SaveFavoritePackage(ctx context.Context, favorite *domain.FavoritePackage) error {
	_, err := s.favoritePackages.InsertOne(ctx, favorite)
	if errors.Is(wrap(err), store.ErrDuplicate) {
		return nil
	}
	return wrap(err)
}

func (s *Store) RemoveFavoritePackage(ctx context.Context, userID, packageID string) error {
	_, err := s.favoritePackages.DeleteOne(ctx, bson.M{
		"user_id":    userID,
		"package_id": packageID,
	})
	return wrap(err)
}

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
