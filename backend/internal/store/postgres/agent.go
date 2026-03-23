package postgres

import (
	"context"
	"encoding/json"

	"github.com/babelsuite/babelsuite/internal/domain"
)

func (s *Store) CreateAgent(ctx context.Context, a *domain.Agent) error {
	labels, _ := json.Marshal(a.Labels)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO agents(agent_id,org_id,name,token,platform,backend,capacity,version,labels,last_contact,no_schedule,created_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		a.AgentID, a.OrgID, a.Name, a.Token, a.Platform, a.Backend,
		a.Capacity, a.Version, string(labels), a.LastContact, a.NoSchedule, a.CreatedAt)
	return wrap(err)
}

func (s *Store) ListAgents(ctx context.Context, orgID string) ([]*domain.Agent, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT agent_id,org_id,name,token,platform,backend,capacity,version,labels,last_contact,no_schedule,created_at
		FROM agents WHERE org_id=$1 ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, wrap(err)
	}
	defer rows.Close()
	var list []*domain.Agent
	for rows.Next() {
		a, err := scanAgent(rows.Scan)
		if err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, nil
}

func (s *Store) GetAgent(ctx context.Context, id string) (*domain.Agent, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT agent_id,org_id,name,token,platform,backend,capacity,version,labels,last_contact,no_schedule,created_at
		FROM agents WHERE agent_id=$1`, id)
	a, err := scanAgent(row.Scan)
	return a, wrap(err)
}

func (s *Store) GetAgentByToken(ctx context.Context, token string) (*domain.Agent, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT agent_id,org_id,name,token,platform,backend,capacity,version,labels,last_contact,no_schedule,created_at
		FROM agents WHERE token=$1`, token)
	a, err := scanAgent(row.Scan)
	return a, wrap(err)
}

func (s *Store) UpdateAgent(ctx context.Context, a *domain.Agent) error {
	labels, _ := json.Marshal(a.Labels)
	_, err := s.pool.Exec(ctx, `
		UPDATE agents SET name=$2,platform=$3,backend=$4,capacity=$5,version=$6,labels=$7,last_contact=$8,no_schedule=$9
		WHERE agent_id=$1`,
		a.AgentID, a.Name, a.Platform, a.Backend, a.Capacity,
		a.Version, string(labels), a.LastContact, a.NoSchedule)
	return wrap(err)
}

func (s *Store) DeleteAgent(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM agents WHERE agent_id=$1`, id)
	return wrap(err)
}

type scanFunc func(dest ...any) error

func scanAgent(scan scanFunc) (*domain.Agent, error) {
	var a domain.Agent
	var labelsJSON string
	err := scan(&a.AgentID, &a.OrgID, &a.Name, &a.Token, &a.Platform, &a.Backend,
		&a.Capacity, &a.Version, &labelsJSON, &a.LastContact, &a.NoSchedule, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	if labelsJSON != "" && labelsJSON != "{}" {
		_ = json.Unmarshal([]byte(labelsJSON), &a.Labels)
	}
	return &a, nil
}
