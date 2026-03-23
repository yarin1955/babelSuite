package mongo

import (
	"context"

	"github.com/babelsuite/babelsuite/internal/domain"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func (s *Store) CreateAgent(ctx context.Context, a *domain.Agent) error {
	_, err := s.agents.InsertOne(ctx, a)
	return wrap(err)
}

func (s *Store) ListAgents(ctx context.Context, orgID string) ([]*domain.Agent, error) {
	cur, err := s.agents.Find(ctx, bson.M{"org_id": orgID})
	if err != nil {
		return nil, wrap(err)
	}
	defer cur.Close(ctx)
	var list []*domain.Agent
	if err := cur.All(ctx, &list); err != nil {
		return nil, err
	}
	return list, nil
}

func (s *Store) GetAgent(ctx context.Context, id string) (*domain.Agent, error) {
	var a domain.Agent
	return &a, wrap(s.agents.FindOne(ctx, bson.M{"agent_id": id}).Decode(&a))
}

func (s *Store) GetAgentByToken(ctx context.Context, token string) (*domain.Agent, error) {
	var a domain.Agent
	return &a, wrap(s.agents.FindOne(ctx, bson.M{"token": token}).Decode(&a))
}

func (s *Store) UpdateAgent(ctx context.Context, a *domain.Agent) error {
	_, err := s.agents.ReplaceOne(ctx, bson.M{"agent_id": a.AgentID}, a)
	return wrap(err)
}

func (s *Store) DeleteAgent(ctx context.Context, id string) error {
	_, err := s.agents.DeleteOne(ctx, bson.M{"agent_id": id})
	return wrap(err)
}
