package store

import (
	"context"
	"time"

	"github.com/babelsuite/babelsuite/internal/cachehub"
	"github.com/babelsuite/babelsuite/internal/domain"
)

type CacheConfig struct {
	WorkspaceTTL time.Duration
	FavoritesTTL time.Duration
}

type cacheStore struct {
	base   Store
	hub    *cachehub.Hub
	config CacheConfig
}

func WithRedis(base Store, hub *cachehub.Hub, config CacheConfig) Store {
	if base == nil || hub == nil || !hub.Enabled() {
		return base
	}
	return &cacheStore{
		base:   base,
		hub:    hub,
		config: config,
	}
}

func (s *cacheStore) CreateWorkspace(ctx context.Context, workspace *domain.Workspace) error {
	if err := s.base.CreateWorkspace(ctx, workspace); err != nil {
		return err
	}
	s.cacheWorkspace(ctx, workspace)
	return nil
}

func (s *cacheStore) DeleteWorkspace(ctx context.Context, id string) error {
	workspace, _ := s.GetWorkspaceByID(ctx, id)
	if err := s.base.DeleteWorkspace(ctx, id); err != nil {
		return err
	}

	_ = s.hub.Remove(ctx, s.workspaceIDKey(id))
	if workspace != nil {
		_ = s.hub.Remove(ctx, s.workspaceSlugKey(workspace.Slug))
	}
	return nil
}

func (s *cacheStore) GetWorkspaceByID(ctx context.Context, id string) (*domain.Workspace, error) {
	var cached domain.Workspace
	if ok, err := s.hub.ReadJSON(ctx, s.workspaceIDKey(id), &cached); err == nil && ok {
		return &cached, nil
	}

	workspace, err := s.base.GetWorkspaceByID(ctx, id)
	if err != nil {
		return nil, err
	}
	s.cacheWorkspace(ctx, workspace)
	return workspace, nil
}

func (s *cacheStore) GetWorkspaceBySlug(ctx context.Context, slug string) (*domain.Workspace, error) {
	var cached domain.Workspace
	if ok, err := s.hub.ReadJSON(ctx, s.workspaceSlugKey(slug), &cached); err == nil && ok {
		return &cached, nil
	}

	workspace, err := s.base.GetWorkspaceBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	s.cacheWorkspace(ctx, workspace)
	return workspace, nil
}

func (s *cacheStore) CreateUser(ctx context.Context, user *domain.User) error {
	return s.base.CreateUser(ctx, user)
}

func (s *cacheStore) GetUserByID(ctx context.Context, id string) (*domain.User, error) {
	return s.base.GetUserByID(ctx, id)
}

func (s *cacheStore) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	return s.base.GetUserByEmail(ctx, email)
}

func (s *cacheStore) GetUserByUsername(ctx context.Context, username string) (*domain.User, error) {
	return s.base.GetUserByUsername(ctx, username)
}

func (s *cacheStore) ListFavoritePackageIDs(ctx context.Context, userID string) ([]string, error) {
	var cached []string
	if ok, err := s.hub.ReadJSON(ctx, s.favoritesKey(userID), &cached); err == nil && ok {
		return append([]string{}, cached...), nil
	}

	packageIDs, err := s.base.ListFavoritePackageIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	_ = s.hub.WriteJSON(ctx, s.favoritesKey(userID), packageIDs, s.config.FavoritesTTL)
	return packageIDs, nil
}

func (s *cacheStore) SaveFavoritePackage(ctx context.Context, favorite *domain.FavoritePackage) error {
	if err := s.base.SaveFavoritePackage(ctx, favorite); err != nil {
		return err
	}
	_ = s.hub.Remove(ctx, s.favoritesKey(favorite.UserID))
	return nil
}

func (s *cacheStore) RemoveFavoritePackage(ctx context.Context, userID, packageID string) error {
	if err := s.base.RemoveFavoritePackage(ctx, userID, packageID); err != nil {
		return err
	}
	_ = s.hub.Remove(ctx, s.favoritesKey(userID))
	return nil
}

func (s *cacheStore) Close(ctx context.Context) error {
	return s.base.Close(ctx)
}

func (s *cacheStore) cacheWorkspace(ctx context.Context, workspace *domain.Workspace) {
	if workspace == nil {
		return
	}
	_ = s.hub.WriteJSON(ctx, s.workspaceIDKey(workspace.WorkspaceID), workspace, s.config.WorkspaceTTL)
	_ = s.hub.WriteJSON(ctx, s.workspaceSlugKey(workspace.Slug), workspace, s.config.WorkspaceTTL)
}

func (s *cacheStore) workspaceIDKey(id string) string {
	return s.hub.Key("workspace", "id", id)
}

func (s *cacheStore) workspaceSlugKey(slug string) string {
	return s.hub.Key("workspace", "slug", slug)
}

func (s *cacheStore) favoritesKey(userID string) string {
	return s.hub.Key("favorites", "user", userID)
}
