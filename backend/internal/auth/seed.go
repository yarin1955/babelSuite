package auth

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/babelsuite/babelsuite/internal/domain"
	"github.com/babelsuite/babelsuite/internal/store"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func Seed(ctx context.Context, st store.Store, email, password string) {
	email = strings.TrimSpace(strings.ToLower(email))
	password = strings.TrimSpace(password)
	if email == "" || password == "" {
		return
	}

	if _, err := st.GetUserByEmail(ctx, email); err == nil {
		return
	} else if !errors.Is(err, store.ErrNotFound) {
		log.Printf("seed: check email: %v", err)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("seed: hash password: %v", err)
		return
	}

	baseUsername := usernameBase("", email)
	if baseUsername == "" {
		baseUsername = "admin"
	}

	workspace, err := createSeedWorkspace(ctx, st, baseUsername)
	if err != nil {
		log.Printf("seed: create workspace: %v", err)
		return
	}

	for attempt := 0; attempt < 5; attempt++ {
		loginUsername := baseUsername
		if attempt > 0 {
			loginUsername = loginUsername + "-" + uuid.NewString()[:6]
		}

		user := &domain.User{
			UserID:      uuid.NewString(),
			WorkspaceID: workspace.WorkspaceID,
			Username:    loginUsername,
			Email:       email,
			FullName:    "Administrator",
			IsAdmin:     true,
			PassHash:    string(hash),
			CreatedAt:   time.Now().UTC(),
		}

		if err := st.CreateUser(ctx, user); err != nil {
			if errors.Is(err, store.ErrDuplicate) {
				continue
			}
			log.Printf("seed: create user: %v", err)
			return
		}

		log.Printf("seed: admin account %q created", email)
		return
	}

	log.Printf("seed: create user: %v", store.ErrDuplicate)
}

func createSeedWorkspace(ctx context.Context, st store.Store, username string) (*domain.Workspace, error) {
	baseSlug := username
	if baseSlug == "" {
		baseSlug = "admin"
	}

	for attempt := 0; attempt < 5; attempt++ {
		slug := baseSlug
		if attempt > 0 {
			slug = slug + "-" + uuid.NewString()[:6]
		}

		workspace := &domain.Workspace{
			WorkspaceID: uuid.NewString(),
			Slug:        slug,
			Name:        "Admin workspace",
			CreatedAt:   time.Now().UTC(),
		}
		if err := st.CreateWorkspace(ctx, workspace); err != nil {
			if errors.Is(err, store.ErrDuplicate) {
				continue
			}
			return nil, err
		}
		return workspace, nil
	}

	return nil, store.ErrDuplicate
}
