package postgres

import (
	"context"
	"encoding/json"

	"github.com/babelsuite/babelsuite/internal/domain"
)

func (s *Store) CreateAgent(ctx context.Context, a *domain.Agent) error {
	labels, _ := json.Marshal(a.Labels)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO agents(agent_id,org_id,name,token,runtime_target_id,desired_backend,desired_platform,desired_target_name,desired_target_url,platform,backend,target_name,target_url,capacity,version,labels,last_contact,last_work,no_schedule,created_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)`,
		a.AgentID, a.OrgID, a.Name, a.Token, a.RuntimeTargetID, a.DesiredBackend, a.DesiredPlatform, a.DesiredTargetName, a.DesiredTargetURL, a.Platform, a.Backend,
		a.TargetName, a.TargetURL, a.Capacity, a.Version, string(labels), a.LastContact, a.LastWork, a.NoSchedule, a.CreatedAt)
	return wrap(err)
}

func (s *Store) ListAgents(ctx context.Context, orgID string) ([]*domain.Agent, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT agent_id,org_id,name,token,runtime_target_id,desired_backend,desired_platform,desired_target_name,desired_target_url,platform,backend,target_name,target_url,capacity,version,labels,last_contact,last_work,no_schedule,created_at
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
		SELECT agent_id,org_id,name,token,runtime_target_id,desired_backend,desired_platform,desired_target_name,desired_target_url,platform,backend,target_name,target_url,capacity,version,labels,last_contact,last_work,no_schedule,created_at
		FROM agents WHERE agent_id=$1`, id)
	a, err := scanAgent(row.Scan)
	return a, wrap(err)
}

func (s *Store) GetAgentByToken(ctx context.Context, token string) (*domain.Agent, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT agent_id,org_id,name,token,runtime_target_id,desired_backend,desired_platform,desired_target_name,desired_target_url,platform,backend,target_name,target_url,capacity,version,labels,last_contact,last_work,no_schedule,created_at
		FROM agents WHERE token=$1`, token)
	a, err := scanAgent(row.Scan)
	return a, wrap(err)
}

func (s *Store) UpdateAgent(ctx context.Context, a *domain.Agent) error {
	labels, _ := json.Marshal(a.Labels)
	_, err := s.pool.Exec(ctx, `
		UPDATE agents SET name=$2,runtime_target_id=$3,desired_backend=$4,desired_platform=$5,desired_target_name=$6,desired_target_url=$7,platform=$8,backend=$9,target_name=$10,target_url=$11,capacity=$12,version=$13,labels=$14,last_contact=$15,last_work=$16,no_schedule=$17
		WHERE agent_id=$1`,
		a.AgentID, a.Name, a.RuntimeTargetID, a.DesiredBackend, a.DesiredPlatform, a.DesiredTargetName, a.DesiredTargetURL, a.Platform, a.Backend, a.TargetName, a.TargetURL, a.Capacity,
		a.Version, string(labels), a.LastContact, a.LastWork, a.NoSchedule)
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
	err := scan(&a.AgentID, &a.OrgID, &a.Name, &a.Token, &a.RuntimeTargetID, &a.DesiredBackend, &a.DesiredPlatform, &a.DesiredTargetName, &a.DesiredTargetURL, &a.Platform, &a.Backend, &a.TargetName, &a.TargetURL,
		&a.Capacity, &a.Version, &labelsJSON, &a.LastContact, &a.LastWork, &a.NoSchedule, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	if labelsJSON != "" && labelsJSON != "{}" {
		_ = json.Unmarshal([]byte(labelsJSON), &a.Labels)
	}
	return &a, nil
}
