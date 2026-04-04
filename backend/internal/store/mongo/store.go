package mongo

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/babelsuite/babelsuite/internal/agent"
	"github.com/babelsuite/babelsuite/internal/domain"
	"github.com/babelsuite/babelsuite/internal/execution"
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
	runtimeDocuments *mongo.Collection
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
		runtimeDocuments: db.Collection("runtime_documents"),
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
	_, _ = st.runtimeDocuments.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "key", Value: 1}}, Options: unique})

	return st, nil
}

func (s *Store) Close(ctx context.Context) error {
	return s.client.Disconnect(ctx)
}

func (s *Store) CreateWorkspace(ctx context.Context, workspace *domain.Workspace) error {
	_, err := s.workspaces.InsertOne(ctx, workspace)
	return wrap(err)
}

func (s *Store) DeleteWorkspace(ctx context.Context, id string) error {
	_, err := s.workspaces.DeleteOne(ctx, bson.M{"workspace_id": id})
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

func (s *Store) LoadAgentRuntime(ctx context.Context) (*agent.RuntimeState, error) {
	var state agent.RuntimeState
	ok, err := s.loadRuntimeDocument(ctx, "agent-runtime", &state)
	if err != nil {
		return nil, err
	}
	if !ok {
		return &agent.RuntimeState{}, nil
	}
	return &state, nil
}

func (s *Store) SaveAgentRuntime(ctx context.Context, state *agent.RuntimeState) error {
	if state == nil {
		state = &agent.RuntimeState{}
	}
	return s.saveRuntimeDocument(ctx, "agent-runtime", state)
}

func (s *Store) LoadAssignmentRuntime(ctx context.Context) ([]agent.AssignmentSnapshot, error) {
	var snapshots []agent.AssignmentSnapshot
	ok, err := s.loadRuntimeDocument(ctx, "assignment-runtime", &snapshots)
	if err != nil {
		return nil, err
	}
	if !ok {
		return []agent.AssignmentSnapshot{}, nil
	}
	return snapshots, nil
}

func (s *Store) SaveAssignmentRuntime(ctx context.Context, snapshots []agent.AssignmentSnapshot) error {
	if snapshots == nil {
		snapshots = []agent.AssignmentSnapshot{}
	}
	return s.saveRuntimeDocument(ctx, "assignment-runtime", snapshots)
}

func (s *Store) LoadExecutionRuntime(ctx context.Context) ([]execution.PersistedExecution, error) {
	var persisted []execution.PersistedExecution
	ok, err := s.loadRuntimeDocument(ctx, "execution-runtime", &persisted)
	if err != nil {
		return nil, err
	}
	if !ok {
		return []execution.PersistedExecution{}, nil
	}
	return persisted, nil
}

func (s *Store) SaveExecutionRuntime(ctx context.Context, persisted []execution.PersistedExecution) error {
	if persisted == nil {
		persisted = []execution.PersistedExecution{}
	}
	return s.saveRuntimeDocument(ctx, "execution-runtime", persisted)
}

func (s *Store) loadRuntimeDocument(ctx context.Context, key string, target any) (bool, error) {
	var document struct {
		Payload string `bson:"payload"`
	}
	err := s.runtimeDocuments.FindOne(ctx, bson.M{"key": key}).Decode(&document)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if document.Payload == "" {
		return true, nil
	}
	if err := json.Unmarshal([]byte(document.Payload), target); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) saveRuntimeDocument(ctx context.Context, key string, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}

	_, err = s.runtimeDocuments.UpdateOne(ctx,
		bson.M{"key": key},
		bson.M{"$set": bson.M{
			"key":        key,
			"payload":    string(payload),
			"updated_at": time.Now().UTC(),
		}},
		options.UpdateOne().SetUpsert(true),
	)
	return err
}
