package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/babelsuite/babelsuite/internal/agent"
	"github.com/babelsuite/babelsuite/internal/domain"
	"github.com/babelsuite/babelsuite/internal/execution"
	"github.com/babelsuite/babelsuite/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func New(dsn string) (*Store, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, err
	}

	st := &Store{pool: pool}
	if err := st.migrate(ctx); err != nil {
		return nil, err
	}
	return st, nil
}

func (s *Store) Close(_ context.Context) error {
	s.pool.Close()
	return nil
}

func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *Store) migrate(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS workspaces (
  workspace_id TEXT PRIMARY KEY,
  slug TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS users (
  user_id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(workspace_id),
  username TEXT NOT NULL UNIQUE,
  email TEXT NOT NULL UNIQUE,
  full_name TEXT NOT NULL,
  is_admin BOOLEAN NOT NULL DEFAULT false,
  pass_hash TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS favorite_packages (
  user_id TEXT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
  package_id TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, package_id)
);

CREATE TABLE IF NOT EXISTS runtime_documents (
  key TEXT PRIMARY KEY,
  payload JSONB NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
`)
	return err
}

func (s *Store) CreateWorkspace(ctx context.Context, workspace *domain.Workspace) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO workspaces (workspace_id, slug, name, created_at) VALUES ($1, $2, $3, $4)`,
		workspace.WorkspaceID, workspace.Slug, workspace.Name, workspace.CreatedAt,
	)
	return wrap(err)
}

func (s *Store) DeleteWorkspace(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM workspaces WHERE workspace_id = $1`, id)
	return wrap(err)
}

func (s *Store) GetWorkspaceByID(ctx context.Context, id string) (*domain.Workspace, error) {
	var workspace domain.Workspace
	err := s.pool.QueryRow(ctx,
		`SELECT workspace_id, slug, name, created_at FROM workspaces WHERE workspace_id = $1`,
		id,
	).Scan(&workspace.WorkspaceID, &workspace.Slug, &workspace.Name, &workspace.CreatedAt)
	return &workspace, wrap(err)
}

func (s *Store) GetWorkspaceBySlug(ctx context.Context, slug string) (*domain.Workspace, error) {
	var workspace domain.Workspace
	err := s.pool.QueryRow(ctx,
		`SELECT workspace_id, slug, name, created_at FROM workspaces WHERE slug = $1`,
		slug,
	).Scan(&workspace.WorkspaceID, &workspace.Slug, &workspace.Name, &workspace.CreatedAt)
	return &workspace, wrap(err)
}

func (s *Store) CreateUser(ctx context.Context, user *domain.User) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO users (user_id, workspace_id, username, email, full_name, is_admin, pass_hash, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		user.UserID, user.WorkspaceID, user.Username, user.Email, user.FullName, user.IsAdmin, user.PassHash, user.CreatedAt,
	)
	return wrap(err)
}

func (s *Store) GetUserByID(ctx context.Context, id string) (*domain.User, error) {
	var user domain.User
	err := s.pool.QueryRow(ctx,
		`SELECT user_id, workspace_id, username, email, full_name, is_admin, pass_hash, created_at FROM users WHERE user_id = $1`,
		id,
	).Scan(&user.UserID, &user.WorkspaceID, &user.Username, &user.Email, &user.FullName, &user.IsAdmin, &user.PassHash, &user.CreatedAt)
	return &user, wrap(err)
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	var user domain.User
	err := s.pool.QueryRow(ctx,
		`SELECT user_id, workspace_id, username, email, full_name, is_admin, pass_hash, created_at FROM users WHERE email = $1`,
		email,
	).Scan(&user.UserID, &user.WorkspaceID, &user.Username, &user.Email, &user.FullName, &user.IsAdmin, &user.PassHash, &user.CreatedAt)
	return &user, wrap(err)
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (*domain.User, error) {
	var user domain.User
	err := s.pool.QueryRow(ctx,
		`SELECT user_id, workspace_id, username, email, full_name, is_admin, pass_hash, created_at FROM users WHERE username = $1`,
		username,
	).Scan(&user.UserID, &user.WorkspaceID, &user.Username, &user.Email, &user.FullName, &user.IsAdmin, &user.PassHash, &user.CreatedAt)
	return &user, wrap(err)
}

func (s *Store) ListFavoritePackageIDs(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT package_id FROM favorite_packages WHERE user_id = $1 ORDER BY created_at DESC, package_id ASC`,
		userID,
	)
	if err != nil {
		return nil, wrap(err)
	}
	defer rows.Close()

	packageIDs := make([]string, 0)
	for rows.Next() {
		var packageID string
		if err := rows.Scan(&packageID); err != nil {
			return nil, err
		}
		if packageID != "" {
			packageIDs = append(packageIDs, packageID)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return packageIDs, nil
}

func (s *Store) SaveFavoritePackage(ctx context.Context, favorite *domain.FavoritePackage) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO favorite_packages (user_id, package_id, created_at) VALUES ($1, $2, $3)
		 ON CONFLICT (user_id, package_id) DO NOTHING`,
		favorite.UserID, favorite.PackageID, favorite.CreatedAt,
	)
	return wrap(err)
}

func (s *Store) RemoveFavoritePackage(ctx context.Context, userID, packageID string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM favorite_packages WHERE user_id = $1 AND package_id = $2`,
		userID, packageID,
	)
	return wrap(err)
}

func wrap(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return store.ErrNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
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
	var payload []byte
	err := s.pool.QueryRow(ctx, `SELECT payload FROM runtime_documents WHERE key = $1`, key).Scan(&payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if len(payload) == 0 {
		return true, nil
	}
	if err := json.Unmarshal(payload, target); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) saveRuntimeDocument(ctx context.Context, key string, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
INSERT INTO runtime_documents (key, payload, updated_at)
VALUES ($1, $2::jsonb, now())
ON CONFLICT (key) DO UPDATE SET payload = EXCLUDED.payload, updated_at = EXCLUDED.updated_at
`, key, string(payload))
	return err
}
