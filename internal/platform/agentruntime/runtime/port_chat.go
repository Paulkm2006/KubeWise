package runtime

import (
	"context"

	"github.com/kubewise/kubewise/internal/platform/agentruntime"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/event"
)

var _ agentruntime.ChatPort = (*Runtime)(nil)

func (r *Runtime) HandleQuery(query string) (string, error) {
	return r.Router.HandleQuery(query)
}

func (r *Runtime) HandleQueryStream(ctx context.Context, query, queryID string, eventCh chan<- event.Event) error {
	return r.Router.HandleQueryStream(ctx, query, queryID, eventCh)
}
