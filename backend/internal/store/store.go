package store

import (
	"context"
	"errors"

	"github.com/babelsuite/babelsuite/internal/domain"
)

var (
	ErrNotFound  = errors.New("not found")
	ErrDuplicate = errors.New("already exists")
)

type Store interface {
	CreateWorkspace(ctx context.Context, workspace *domain.Workspace) error
	GetWorkspaceByID(ctx context.Context, id string) (*domain.Workspace, error)
	GetWorkspaceBySlug(ctx context.Context, slug string) (*domain.Workspace, error)

	CreateUser(ctx context.Context, user *domain.User) error
	GetUserByID(ctx context.Context, id string) (*domain.User, error)
	GetUserByEmail(ctx context.Context, email string) (*domain.User, error)
	GetUserByUsername(ctx context.Context, username string) (*domain.User, error)
	ListFavoritePackageIDs(ctx context.Context, userID string) ([]string, error)
	SaveFavoritePackage(ctx context.Context, favorite *domain.FavoritePackage) error
	RemoveFavoritePackage(ctx context.Context, userID, packageID string) error

	Close(ctx context.Context) error
}
