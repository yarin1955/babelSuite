package store

import (
	"context"
	"errors"

	"github.com/babelsuite/babelsuite/internal/domain"
)

var ErrNotFound = errors.New("not found")
var ErrDuplicate = errors.New("already exists")

type Store interface {
	// Orgs
	CreateOrg(ctx context.Context, o *domain.Org) error
	GetOrgBySlug(ctx context.Context, slug string) (*domain.Org, error)
	GetOrgByID(ctx context.Context, id string) (*domain.Org, error)

	// Users
	CreateUser(ctx context.Context, u *domain.User) error
	GetUserByID(ctx context.Context, id string) (*domain.User, error)
	GetUserByUsername(ctx context.Context, username string) (*domain.User, error)
	GetUserByEmail(ctx context.Context, email string) (*domain.User, error)

	// Registries
	CreateRegistry(ctx context.Context, r *domain.Registry) error
	ListRegistries(ctx context.Context, orgID string) ([]*domain.Registry, error)
	GetRegistry(ctx context.Context, id string) (*domain.Registry, error)
	UpdateRegistry(ctx context.Context, r *domain.Registry) error
	DeleteRegistry(ctx context.Context, id string) error

	// Catalog packages
	UpsertPackage(ctx context.Context, p *domain.CatalogPackage) error
	ListPackages(ctx context.Context, orgID string, f domain.CatalogFilter) ([]*domain.CatalogPackage, int64, error)
	GetPackage(ctx context.Context, id string) (*domain.CatalogPackage, error)
	SetPackageEnabled(ctx context.Context, id string, enabled bool) error
	DeletePackage(ctx context.Context, id string) error

	// Profiles
	CreateProfile(ctx context.Context, p *domain.Profile) error
	ListProfiles(ctx context.Context, orgID string) ([]*domain.Profile, error)
	GetProfile(ctx context.Context, id string) (*domain.Profile, error)
	UpdateProfile(ctx context.Context, p *domain.Profile) error
	DeleteProfile(ctx context.Context, id string) error

	// Runs
	CreateRun(ctx context.Context, r *domain.Run) error
	ListRuns(ctx context.Context, orgID string, page, pageSize int) ([]*domain.Run, int64, error)
	GetRun(ctx context.Context, id string) (*domain.Run, error)
	UpdateRun(ctx context.Context, r *domain.Run) error
	NextPendingRun(ctx context.Context, orgID, agentID string) (*domain.Run, error)
	CountActiveRunsByAgent(ctx context.Context, agentID string) (int64, error)

	// Steps
	CreateStep(ctx context.Context, s *domain.Step) error
	ListSteps(ctx context.Context, runID string) ([]*domain.Step, error)
	UpdateStep(ctx context.Context, s *domain.Step) error

	// Logs
	AppendLogs(ctx context.Context, entries []*domain.LogEntry) error
	GetLogs(ctx context.Context, stepID string) ([]*domain.LogEntry, error)

	// Agents
	CreateAgent(ctx context.Context, a *domain.Agent) error
	ListAgents(ctx context.Context, orgID string) ([]*domain.Agent, error)
	GetAgent(ctx context.Context, id string) (*domain.Agent, error)
	GetAgentByToken(ctx context.Context, token string) (*domain.Agent, error)
	UpdateAgent(ctx context.Context, a *domain.Agent) error
	DeleteAgent(ctx context.Context, id string) error

	// OIDC providers
	CreateOIDCProvider(ctx context.Context, p *domain.OIDCProvider) error
	ListOIDCProviders(ctx context.Context) ([]*domain.OIDCProvider, error)
	GetOIDCProvider(ctx context.Context, id string) (*domain.OIDCProvider, error)
	UpdateOIDCProvider(ctx context.Context, p *domain.OIDCProvider) error
	DeleteOIDCProvider(ctx context.Context, id string) error

	// Users (upsert for SSO-created accounts)
	UpsertUserByEmail(ctx context.Context, u *domain.User) (*domain.User, error)

	Close(ctx context.Context) error
}
