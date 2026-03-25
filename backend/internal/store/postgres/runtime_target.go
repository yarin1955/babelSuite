package postgres

import (
	"context"
	"encoding/json"

	"github.com/babelsuite/babelsuite/internal/domain"
)

func (s *Store) CreateRuntimeTarget(ctx context.Context, t *domain.RuntimeTarget) error {
	labels, _ := json.Marshal(t.Labels)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO runtime_targets(runtime_target_id,org_id,name,backend,platform,endpoint_url,namespace,insecure_skip_tls_verify,username,password,bearer_token,tls_ca_data,tls_cert_data,tls_key_data,labels,created_at,updated_at)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
		t.RuntimeTargetID, t.OrgID, t.Name, t.Backend, t.Platform, t.EndpointURL, t.Namespace, t.InsecureSkipTLSVerify, t.Username, t.Password, t.BearerToken, t.TLSCAData, t.TLSCertData, t.TLSKeyData, string(labels), t.CreatedAt, t.UpdatedAt,
	)
	return wrap(err)
}

func (s *Store) ListRuntimeTargets(ctx context.Context, orgID string) ([]*domain.RuntimeTarget, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT runtime_target_id,org_id,name,backend,platform,endpoint_url,namespace,insecure_skip_tls_verify,username,password,bearer_token,tls_ca_data,tls_cert_data,tls_key_data,labels,created_at,updated_at
		 FROM runtime_targets WHERE org_id=$1 ORDER BY name ASC`, orgID)
	if err != nil {
		return nil, wrap(err)
	}
	defer rows.Close()

	var list []*domain.RuntimeTarget
	for rows.Next() {
		target, err := scanRuntimeTarget(rows.Scan)
		if err != nil {
			return nil, err
		}
		list = append(list, target)
	}
	return list, rows.Err()
}

func (s *Store) GetRuntimeTarget(ctx context.Context, id string) (*domain.RuntimeTarget, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT runtime_target_id,org_id,name,backend,platform,endpoint_url,namespace,insecure_skip_tls_verify,username,password,bearer_token,tls_ca_data,tls_cert_data,tls_key_data,labels,created_at,updated_at
		 FROM runtime_targets WHERE runtime_target_id=$1`, id)
	target, err := scanRuntimeTarget(row.Scan)
	return target, wrap(err)
}

func (s *Store) UpdateRuntimeTarget(ctx context.Context, t *domain.RuntimeTarget) error {
	labels, _ := json.Marshal(t.Labels)
	_, err := s.pool.Exec(ctx,
		`UPDATE runtime_targets
		 SET name=$2,backend=$3,platform=$4,endpoint_url=$5,namespace=$6,insecure_skip_tls_verify=$7,username=$8,password=$9,bearer_token=$10,tls_ca_data=$11,tls_cert_data=$12,tls_key_data=$13,labels=$14,updated_at=$15
		 WHERE runtime_target_id=$1`,
		t.RuntimeTargetID, t.Name, t.Backend, t.Platform, t.EndpointURL, t.Namespace, t.InsecureSkipTLSVerify, t.Username, t.Password, t.BearerToken, t.TLSCAData, t.TLSCertData, t.TLSKeyData, string(labels), t.UpdatedAt,
	)
	return wrap(err)
}

func (s *Store) DeleteRuntimeTarget(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM runtime_targets WHERE runtime_target_id=$1`, id)
	return wrap(err)
}

func scanRuntimeTarget(scan scanFunc) (*domain.RuntimeTarget, error) {
	var target domain.RuntimeTarget
	var labelsJSON string
	err := scan(&target.RuntimeTargetID, &target.OrgID, &target.Name, &target.Backend, &target.Platform, &target.EndpointURL, &target.Namespace, &target.InsecureSkipTLSVerify, &target.Username, &target.Password, &target.BearerToken, &target.TLSCAData, &target.TLSCertData, &target.TLSKeyData, &labelsJSON, &target.CreatedAt, &target.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if labelsJSON != "" && labelsJSON != "{}" {
		_ = json.Unmarshal([]byte(labelsJSON), &target.Labels)
	}
	return &target, nil
}
