package deploy

import (
	"context"
	"fmt"

	"github.com/kubewise/kubewise/pkg/agent/deploy/nodes"
	"github.com/kubewise/kubewise/pkg/agent/deploy/state"
	"github.com/kubewise/kubewise/pkg/types"
)

// RunPipeline executes the deploy state machine until a terminal phase or error.
func RunPipeline(st *state.State) (string, error) {
	for !st.Phase.Terminal() {
		fn, ok := nodes.Dispatch(st.Phase)
		if !ok {
			err := fmt.Errorf("unhandled deploy phase: %s", st.Phase.String())
			st.Fail(err)
			return "", err
		}
		if err := fn(st); err != nil {
			st.Fail(err)
			return "", err
		}
		if st.Phase.Terminal() {
			break
		}
	}
	if st.Err != nil {
		return "", st.Err
	}
	return st.Result, nil
}

func (a *Agent) runDeployPipeline(ctx context.Context, query string, entities types.Entities) (string, error) {
	st := state.New(ctx, query, entities, state.Deps{
		LLM:      a.llmClient,
		Helm:     a.helmClient,
		K8s:      a.k8sClient,
		Tools:    a.toolRegistry,
		Confirm:  a,
		Select:   a,
		BuildReport: a.buildReport,
		QueryID:  a.queryID,
		EventCh:  a.eventCh,
		Log:      a.logger(),
	})
	return RunPipeline(st)
}
