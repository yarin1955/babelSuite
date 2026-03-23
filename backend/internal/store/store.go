package store

import (
	"context"
	"errors"

	"github.com/babelsuite/babelsuite/internal/domain"
)

var ErrNotFound  = errors.New("not found")
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

	Close(ctx context.Context) error
}
