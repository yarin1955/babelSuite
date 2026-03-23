package store

import (
	"context"
	"errors"

	"github.com/babelsuite/babelsuite/internal/domain"
)

var ErrNotFound  = errors.New("not found")
var ErrDuplicate = errors.New("already exists")

type Store interface {
	CreateOrg(ctx context.Context, o *domain.Org) error
	GetOrgBySlug(ctx context.Context, slug string) (*domain.Org, error)
	GetOrgByID(ctx context.Context, id string) (*domain.Org, error)

	CreateUser(ctx context.Context, u *domain.User) error
	GetUserByID(ctx context.Context, id string) (*domain.User, error)
	GetUserByUsername(ctx context.Context, username string) (*domain.User, error)
	GetUserByEmail(ctx context.Context, email string) (*domain.User, error)

	Close(ctx context.Context) error
}
