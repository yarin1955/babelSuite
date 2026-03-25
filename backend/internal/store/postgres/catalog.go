package postgres

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/babelsuite/babelsuite/internal/domain"
	"github.com/jackc/pgx/v5"
)

func (s *Store) migratecatalog(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS registries (
  registry_id TEXT PRIMARY KEY,
  org_id      TEXT NOT NULL REFERENCES orgs(org_id),
  kind        TEXT NOT NULL,
  name        TEXT NOT NULL,
  url         TEXT,
  token       TEXT,
  enabled     BOOLEAN NOT NULL DEFAULT true,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS catalog_packages (
  package_id    TEXT PRIMARY KEY,
  org_id        TEXT NOT NULL REFERENCES orgs(org_id),
  registry_id   TEXT NOT NULL,
  registry_kind TEXT NOT NULL,
  name          TEXT NOT NULL,
  display_name  TEXT,
  description   TEXT,
  publisher     TEXT,
  image_ref     TEXT NOT NULL,
  version       TEXT,
  tags          TEXT[],
  profiles      TEXT[],
  default_profile TEXT,
  enabled       BOOLEAN NOT NULL DEFAULT false,
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(org_id, image_ref)
);
ALTER TABLE catalog_packages ADD COLUMN IF NOT EXISTS profiles TEXT[];
ALTER TABLE catalog_packages ADD COLUMN IF NOT EXISTS default_profile TEXT;`)
	return err
}

// ── Registries ────────────────────────────────────────────────────────────────

func (s *Store) CreateRegistry(ctx context.Context, r *domain.Registry) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO registries(registry_id,org_id,kind,name,url,token,enabled,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
		r.RegistryID, r.OrgID, r.Kind, r.Name, r.URL, r.Token, r.Enabled, r.CreatedAt)
	return wrap(err)
}

func (s *Store) ListRegistries(ctx context.Context, orgID string) ([]*domain.Registry, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT registry_id,org_id,kind,name,url,token,enabled,created_at FROM registries WHERE org_id=$1 ORDER BY created_at`, orgID)
	if err != nil {
		return nil, wrap(err)
	}
	defer rows.Close()
	var out []*domain.Registry
	for rows.Next() {
		var r domain.Registry
		if err := rows.Scan(&r.RegistryID, &r.OrgID, &r.Kind, &r.Name, &r.URL, &r.Token, &r.Enabled, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &r)
	}
	return out, rows.Err()
}

func (s *Store) GetRegistry(ctx context.Context, id string) (*domain.Registry, error) {
	var r domain.Registry
	err := s.pool.QueryRow(ctx,
		`SELECT registry_id,org_id,kind,name,url,token,enabled,created_at FROM registries WHERE registry_id=$1`, id).
		Scan(&r.RegistryID, &r.OrgID, &r.Kind, &r.Name, &r.URL, &r.Token, &r.Enabled, &r.CreatedAt)
	return &r, wrap(err)
}

func (s *Store) UpdateRegistry(ctx context.Context, r *domain.Registry) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE registries SET kind=$1,name=$2,url=$3,token=$4,enabled=$5 WHERE registry_id=$6`,
		r.Kind, r.Name, r.URL, r.Token, r.Enabled, r.RegistryID)
	return wrap(err)
}

func (s *Store) DeleteRegistry(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM registries WHERE registry_id=$1`, id)
	return wrap(err)
}

// ── Catalog packages ──────────────────────────────────────────────────────────

func (s *Store) UpsertPackage(ctx context.Context, p *domain.CatalogPackage) error {
	_, err := s.pool.Exec(ctx, `
INSERT INTO catalog_packages(package_id,org_id,registry_id,registry_kind,name,display_name,description,publisher,image_ref,version,tags,profiles,default_profile,enabled,updated_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
ON CONFLICT(org_id,image_ref) DO UPDATE SET
  name=$5, display_name=$6, description=$7, publisher=$8, version=$10, tags=$11, profiles=$12, default_profile=$13, updated_at=$15`,
		p.PackageID, p.OrgID, p.RegistryID, p.RegistryKind, p.Name, p.DisplayName,
		p.Description, p.Publisher, p.ImageRef, p.Version, p.Tags, p.Profiles, p.DefaultProfile, p.Enabled, p.UpdatedAt)
	return wrap(err)
}

func (s *Store) ListPackages(ctx context.Context, orgID string, f domain.CatalogFilter) ([]*domain.CatalogPackage, int64, error) {
	where := []string{"org_id=$1"}
	args := []any{orgID}
	n := 2

	if f.EnabledOnly {
		where = append(where, fmt.Sprintf("enabled=$%d", n))
		args = append(args, true)
		n++
	}
	if f.RegistryKind != "" {
		where = append(where, fmt.Sprintf("registry_kind=$%d", n))
		args = append(args, string(f.RegistryKind))
		n++
	}
	if f.Search != "" {
		where = append(where, fmt.Sprintf("(name ILIKE $%d OR display_name ILIKE $%d OR description ILIKE $%d OR publisher ILIKE $%d OR array_to_string(tags, ',') ILIKE $%d OR array_to_string(profiles, ',') ILIKE $%d)", n, n, n, n, n, n))
		args = append(args, "%"+f.Search+"%")
		n++
	}

	clause := "WHERE " + strings.Join(where, " AND ")

	var total int64
	if err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM catalog_packages "+clause, args...).Scan(&total); err != nil {
		return nil, 0, wrap(err)
	}

	pageSize := f.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	page := f.Page
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * pageSize
	args = append(args, pageSize, offset)

	rows, err := s.pool.Query(ctx,
		`SELECT package_id,org_id,registry_id,registry_kind,name,display_name,description,publisher,image_ref,version,tags,profiles,default_profile,enabled,updated_at
		 FROM catalog_packages `+clause+fmt.Sprintf(" ORDER BY updated_at DESC LIMIT $%d OFFSET $%d", n, n+1),
		args...)
	if err != nil {
		return nil, 0, wrap(err)
	}
	defer rows.Close()

	var out []*domain.CatalogPackage
	for rows.Next() {
		var p domain.CatalogPackage
		if err := rows.Scan(&p.PackageID, &p.OrgID, &p.RegistryID, &p.RegistryKind,
			&p.Name, &p.DisplayName, &p.Description, &p.Publisher,
			&p.ImageRef, &p.Version, &p.Tags, &p.Profiles, &p.DefaultProfile, &p.Enabled, &p.UpdatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, &p)
	}
	return out, total, rows.Err()
}

func (s *Store) GetPackage(ctx context.Context, id string) (*domain.CatalogPackage, error) {
	var p domain.CatalogPackage
	err := s.pool.QueryRow(ctx,
		`SELECT package_id,org_id,registry_id,registry_kind,name,display_name,description,publisher,image_ref,version,tags,profiles,default_profile,enabled,updated_at
		 FROM catalog_packages WHERE package_id=$1`, id).
		Scan(&p.PackageID, &p.OrgID, &p.RegistryID, &p.RegistryKind,
			&p.Name, &p.DisplayName, &p.Description, &p.Publisher,
			&p.ImageRef, &p.Version, &p.Tags, &p.Profiles, &p.DefaultProfile, &p.Enabled, &p.UpdatedAt)
	return &p, wrap(err)
}

func (s *Store) SetPackageEnabled(ctx context.Context, id string, enabled bool) error {
	_, err := s.pool.Exec(ctx, `UPDATE catalog_packages SET enabled=$1 WHERE package_id=$2`, enabled, id)
	return wrap(err)
}

func (s *Store) DeletePackage(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM catalog_packages WHERE package_id=$1`, id)
	return wrap(err)
}

func TotalPages(total int64, pageSize int) int {
	if pageSize <= 0 {
		return 1
	}
	return int(math.Ceil(float64(total) / float64(pageSize)))
}

var _ pgx.Row = nil // keep import used
