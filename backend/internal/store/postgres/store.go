package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/babelsuite/babelsuite/internal/domain"
	"github.com/babelsuite/babelsuite/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

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
	s := &Store{pool: pool}
	return s, s.migrate(ctx)
}

func (s *Store) Close(_ context.Context) error { s.pool.Close(); return nil }

func wrap(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return store.ErrNotFound
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return store.ErrDuplicate
	}
	return err
}

func (s *Store) migrate(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS orgs (
  org_id     TEXT PRIMARY KEY,
  slug       TEXT UNIQUE NOT NULL,
  name       TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS users (
  user_id    TEXT PRIMARY KEY,
  org_id     TEXT NOT NULL REFERENCES orgs(org_id),
  username   TEXT UNIQUE NOT NULL,
  email      TEXT UNIQUE NOT NULL,
  name       TEXT NOT NULL,
  is_admin   BOOLEAN NOT NULL DEFAULT false,
  pass_hash  TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_admin BOOLEAN NOT NULL DEFAULT false;
CREATE TABLE IF NOT EXISTS agents (
  agent_id     TEXT PRIMARY KEY,
  org_id       TEXT NOT NULL,
  name         TEXT NOT NULL,
  token        TEXT UNIQUE NOT NULL,
  platform     TEXT NOT NULL DEFAULT '',
  backend      TEXT NOT NULL DEFAULT '',
  capacity     INT  NOT NULL DEFAULT 1,
  version      TEXT NOT NULL DEFAULT '',
  labels       TEXT NOT NULL DEFAULT '{}',
  last_contact TIMESTAMPTZ NOT NULL DEFAULT now(),
  no_schedule  BOOLEAN NOT NULL DEFAULT false,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS oidc_providers (
  provider_id   TEXT PRIMARY KEY,
  name          TEXT NOT NULL,
  issuer_url    TEXT NOT NULL,
  client_id     TEXT NOT NULL,
  client_secret TEXT NOT NULL,
  scopes        TEXT NOT NULL DEFAULT '',
  enabled       BOOLEAN NOT NULL DEFAULT true,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);`)
	return err
}

func (s *Store) CreateOrg(ctx context.Context, o *domain.Org) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO orgs(org_id,slug,name,created_at) VALUES($1,$2,$3,$4)`,
		o.OrgID, o.Slug, o.Name, o.CreatedAt)
	return wrap(err)
}

func (s *Store) GetOrgBySlug(ctx context.Context, slug string) (*domain.Org, error) {
	var o domain.Org
	err := s.pool.QueryRow(ctx, `SELECT org_id,slug,name,created_at FROM orgs WHERE slug=$1`, slug).
		Scan(&o.OrgID, &o.Slug, &o.Name, &o.CreatedAt)
	return &o, wrap(err)
}

func (s *Store) GetOrgByID(ctx context.Context, id string) (*domain.Org, error) {
	var o domain.Org
	err := s.pool.QueryRow(ctx, `SELECT org_id,slug,name,created_at FROM orgs WHERE org_id=$1`, id).
		Scan(&o.OrgID, &o.Slug, &o.Name, &o.CreatedAt)
	return &o, wrap(err)
}

func (s *Store) CreateUser(ctx context.Context, u *domain.User) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO users(user_id,org_id,username,email,name,is_admin,pass_hash,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
		u.UserID, u.OrgID, u.Username, u.Email, u.Name, u.IsAdmin, u.PassHash, u.CreatedAt)
	return wrap(err)
}

func (s *Store) GetUserByID(ctx context.Context, id string) (*domain.User, error) {
	var u domain.User
	err := s.pool.QueryRow(ctx, `SELECT user_id,org_id,username,email,name,is_admin,pass_hash,created_at FROM users WHERE user_id=$1`, id).
		Scan(&u.UserID, &u.OrgID, &u.Username, &u.Email, &u.Name, &u.IsAdmin, &u.PassHash, &u.CreatedAt)
	return &u, wrap(err)
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (*domain.User, error) {
	var u domain.User
	err := s.pool.QueryRow(ctx, `SELECT user_id,org_id,username,email,name,is_admin,pass_hash,created_at FROM users WHERE username=$1`, username).
		Scan(&u.UserID, &u.OrgID, &u.Username, &u.Email, &u.Name, &u.IsAdmin, &u.PassHash, &u.CreatedAt)
	return &u, wrap(err)
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	var u domain.User
	err := s.pool.QueryRow(ctx, `SELECT user_id,org_id,username,email,name,is_admin,pass_hash,created_at FROM users WHERE email=$1`, email).
		Scan(&u.UserID, &u.OrgID, &u.Username, &u.Email, &u.Name, &u.IsAdmin, &u.PassHash, &u.CreatedAt)
	return &u, wrap(err)
}
