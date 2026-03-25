package postgres

import (
	"context"

	"github.com/babelsuite/babelsuite/internal/domain"
)

func (s *Store) CreateProfile(ctx context.Context, p *domain.Profile) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO profiles(profile_id,org_id,name,description,format,content,revision,created_by,created_by_name,updated_by,updated_by_name,created_at,updated_at)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		p.ProfileID, p.OrgID, p.Name, p.Description, p.Format, p.Content, p.Revision,
		p.CreatedBy, p.CreatedByName, p.UpdatedBy, p.UpdatedByName, p.CreatedAt, p.UpdatedAt,
	)
	return wrap(err)
}

func (s *Store) ListProfiles(ctx context.Context, orgID string) ([]*domain.Profile, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT profile_id,org_id,name,description,format,content,revision,created_by,created_by_name,updated_by,updated_by_name,created_at,updated_at
		 FROM profiles WHERE org_id=$1 ORDER BY updated_at DESC`, orgID)
	if err != nil {
		return nil, wrap(err)
	}
	defer rows.Close()

	var list []*domain.Profile
	for rows.Next() {
		var p domain.Profile
		if err := rows.Scan(&p.ProfileID, &p.OrgID, &p.Name, &p.Description, &p.Format, &p.Content, &p.Revision,
			&p.CreatedBy, &p.CreatedByName, &p.UpdatedBy, &p.UpdatedByName, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, &p)
	}
	return list, rows.Err()
}

func (s *Store) GetProfile(ctx context.Context, id string) (*domain.Profile, error) {
	var p domain.Profile
	err := s.pool.QueryRow(ctx,
		`SELECT profile_id,org_id,name,description,format,content,revision,created_by,created_by_name,updated_by,updated_by_name,created_at,updated_at
		 FROM profiles WHERE profile_id=$1`, id).
		Scan(&p.ProfileID, &p.OrgID, &p.Name, &p.Description, &p.Format, &p.Content, &p.Revision,
			&p.CreatedBy, &p.CreatedByName, &p.UpdatedBy, &p.UpdatedByName, &p.CreatedAt, &p.UpdatedAt)
	return &p, wrap(err)
}

func (s *Store) UpdateProfile(ctx context.Context, p *domain.Profile) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE profiles SET name=$2,description=$3,format=$4,content=$5,revision=$6,updated_by=$7,updated_by_name=$8,updated_at=$9
		 WHERE profile_id=$1`,
		p.ProfileID, p.Name, p.Description, p.Format, p.Content, p.Revision, p.UpdatedBy, p.UpdatedByName, p.UpdatedAt,
	)
	return wrap(err)
}

func (s *Store) DeleteProfile(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM profiles WHERE profile_id=$1`, id)
	return wrap(err)
}
