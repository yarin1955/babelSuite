package auth

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/babelsuite/babelsuite/internal/domain"
	"github.com/babelsuite/babelsuite/internal/store"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// Seed creates the admin user (and a personal org for it) if neither the
// username nor the email "admin" already exist in the store.  Call this once
// at server startup with the values from ADMIN_USERNAME / ADMIN_PASSWORD.
func Seed(ctx context.Context, st store.Store, username, password string) {
	if username == "" || password == "" {
		return
	}

	// Already exists?
	if _, err := st.GetUserByUsername(ctx, username); err == nil {
		return
	} else if !errors.Is(err, store.ErrNotFound) {
		log.Printf("seed: checking user: %v", err)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("seed: bcrypt: %v", err)
		return
	}

	org := &domain.Org{
		OrgID:     uuid.NewString(),
		Slug:      username,
		Name:      username + "'s workspace",
		CreatedAt: time.Now().UTC(),
	}
	if err := st.CreateOrg(ctx, org); err != nil && !errors.Is(err, store.ErrDuplicate) {
		log.Printf("seed: create org: %v", err)
		return
	} else if errors.Is(err, store.ErrDuplicate) {
		org.Slug = username + "-" + uuid.NewString()[:6]
		if err := st.CreateOrg(ctx, org); err != nil {
			log.Printf("seed: create org (retry): %v", err)
			return
		}
	}

	user := &domain.User{
		UserID:    uuid.NewString(),
		OrgID:     org.OrgID,
		Username:  username,
		Email:     username + "@localhost",
		Name:      username,
		IsAdmin:   true,
		PassHash:  string(hash),
		CreatedAt: time.Now().UTC(),
	}
	if err := st.CreateUser(ctx, user); err != nil {
		log.Printf("seed: create user: %v", err)
		return
	}

	log.Printf("seed: admin user %q created", username)
}
