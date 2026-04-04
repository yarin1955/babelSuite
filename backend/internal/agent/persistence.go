package agent

import "context"

type AssignmentStore interface {
	LoadAssignmentRuntime(ctx context.Context) ([]AssignmentSnapshot, error)
	SaveAssignmentRuntime(ctx context.Context, snapshots []AssignmentSnapshot) error
}

type RuntimeRepository interface {
	LoadAgentRuntime(ctx context.Context) (*RuntimeState, error)
	SaveAgentRuntime(ctx context.Context, state *RuntimeState) error
}

type DBRuntimeStore struct {
	repository RuntimeRepository
}

func NewDBRuntimeStore(repository RuntimeRepository) *DBRuntimeStore {
	return &DBRuntimeStore{repository: repository}
}

func (s *DBRuntimeStore) Load() (*RuntimeState, error) {
	if s == nil || s.repository == nil {
		return &RuntimeState{}, nil
	}
	return s.repository.LoadAgentRuntime(context.Background())
}

func (s *DBRuntimeStore) Save(state *RuntimeState) error {
	if s == nil || s.repository == nil {
		return nil
	}
	return s.repository.SaveAgentRuntime(context.Background(), state)
}
