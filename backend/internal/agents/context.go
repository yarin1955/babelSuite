package agents

import (
	"context"

	"github.com/babelsuite/babelsuite/internal/domain"
)

type contextKey struct{}

func contextWithAgent(ctx context.Context, a *domain.Agent) context.Context {
	return context.WithValue(ctx, contextKey{}, a)
}

func agentFromContext(ctx context.Context) *domain.Agent {
	a, _ := ctx.Value(contextKey{}).(*domain.Agent)
	return a
}
