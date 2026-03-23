package postgres

import (
	"context"
	"time"

	"github.com/babelsuite/babelsuite/internal/domain"
)

func (s *Store) CreateOIDCProvider(ctx context.Context, p *domain.OIDCProvider) error {
	scopes := scopesToText(p.Scopes)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO oidc_providers(provider_id,name,issuer_url,client_id,client_secret,scopes,enabled,created_at)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
		p.ProviderID, p.Name, p.IssuerURL, p.ClientID, p.ClientSecret, scopes, p.Enabled, p.CreatedAt)
	return wrap(err)
}

func (s *Store) ListOIDCProviders(ctx context.Context) ([]*domain.OIDCProvider, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT provider_id,name,issuer_url,client_id,client_secret,scopes,enabled,created_at FROM oidc_providers ORDER BY name`)
	if err != nil {
		return nil, wrap(err)
	}
	defer rows.Close()
	var list []*domain.OIDCProvider
	for rows.Next() {
		var p domain.OIDCProvider
		var scopes string
		if err := rows.Scan(&p.ProviderID, &p.Name, &p.IssuerURL, &p.ClientID, &p.ClientSecret, &scopes, &p.Enabled, &p.CreatedAt); err != nil {
			return nil, err
		}
		p.Scopes = textToScopes(scopes)
		list = append(list, &p)
	}
	return list, nil
}

func (s *Store) GetOIDCProvider(ctx context.Context, id string) (*domain.OIDCProvider, error) {
	var p domain.OIDCProvider
	var scopes string
	err := s.pool.QueryRow(ctx,
		`SELECT provider_id,name,issuer_url,client_id,client_secret,scopes,enabled,created_at FROM oidc_providers WHERE provider_id=$1`, id).
		Scan(&p.ProviderID, &p.Name, &p.IssuerURL, &p.ClientID, &p.ClientSecret, &scopes, &p.Enabled, &p.CreatedAt)
	p.Scopes = textToScopes(scopes)
	return &p, wrap(err)
}

func (s *Store) UpdateOIDCProvider(ctx context.Context, p *domain.OIDCProvider) error {
	scopes := scopesToText(p.Scopes)
	_, err := s.pool.Exec(ctx,
		`UPDATE oidc_providers SET name=$2,issuer_url=$3,client_id=$4,client_secret=$5,scopes=$6,enabled=$7 WHERE provider_id=$1`,
		p.ProviderID, p.Name, p.IssuerURL, p.ClientID, p.ClientSecret, scopes, p.Enabled)
	return wrap(err)
}

func (s *Store) DeleteOIDCProvider(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM oidc_providers WHERE provider_id=$1`, id)
	return wrap(err)
}

func (s *Store) UpsertUserByEmail(ctx context.Context, u *domain.User) (*domain.User, error) {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO users(user_id,org_id,username,email,name,is_admin,pass_hash,created_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT(email) DO UPDATE SET username=EXCLUDED.username, name=EXCLUDED.name`,
		u.UserID, u.OrgID, u.Username, u.Email, u.Name, u.IsAdmin, u.PassHash, time.Now().UTC())
	if err != nil {
		return nil, wrap(err)
	}
	return s.GetUserByEmail(ctx, u.Email)
}

func scopesToText(scopes []string) string {
	result := ""
	for i, s := range scopes {
		if i > 0 {
			result += ","
		}
		result += s
	}
	return result
}

func textToScopes(text string) []string {
	if text == "" {
		return nil
	}
	var scopes []string
	start := 0
	for i := 0; i <= len(text); i++ {
		if i == len(text) || text[i] == ',' {
			scopes = append(scopes, text[start:i])
			start = i + 1
		}
	}
	return scopes
}
