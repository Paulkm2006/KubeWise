package runtime

import (
	"context"
	"sync"

	"github.com/kubewise/kubewise/internal/platform/agentruntime"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/event"
)

var _ agentruntime.DiagnosisRunner = (*Runtime)(nil)

func (r *Runtime) DiagnosePod(ctx context.Context, params agentruntime.DiagnoseParams, queryID string, out chan<- agentruntime.ProgressEvent) error {
	internalCh := make(chan event.Event, 64)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for ev := range internalCh {
			if pe, ok := toProgressEvent(ev); ok {
				select {
				case out <- pe:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	err := r.Router.DiagnosePodStream(ctx, params, queryID, internalCh)
	close(internalCh)
	wg.Wait()
	return err
}
