package runs

import (
	"context"

	"github.com/babelsuite/babelsuite/internal/domain"
)

type agentCtxKey struct{}

func contextWithAgent(ctx context.Context, a *domain.Agent) context.Context {
	return context.WithValue(ctx, agentCtxKey{}, a)
}

func agentFromCtx(r interface{ Context() context.Context }) *domain.Agent {
	a, _ := r.Context().Value(agentCtxKey{}).(*domain.Agent)
	return a
}
