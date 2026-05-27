package router

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	deploytypes "github.com/kubewise/kubewise/pkg/agent/deploy/types"
	"github.com/kubewise/kubewise/pkg/catalog"
	"github.com/kubewise/kubewise/pkg/stream"
)

// streamChartSelectionHandler sends chart selection as a unified stream interaction.
type streamChartSelectionHandler struct {
	emitter   stream.Emitter
	queryID   string
	bridgeCtx context.Context
}

func (h *streamChartSelectionHandler) SelectChart(ctx context.Context, appName string, candidates []catalog.ChartInfo) (*catalog.ChartInfo, error) {
	payload, err := json.Marshal(struct {
		AppName    string              `json:"app_name"`
		Candidates []catalog.ChartInfo `json:"candidates"`
	}{
		AppName:    appName,
		Candidates: candidates,
	})
	if err != nil {
		return nil, err
	}
	respCh := make(chan json.RawMessage, 1)
	ireq := stream.InteractionRequest{
		QueryID: h.queryID,
		Kind:    stream.KindChartSelect,
		Payload: payload,
		RespCh:  respCh,
	}
	if err := h.emitter.Emit(ctx, ireq); err != nil {
		return nil, err
	}
	select {
	case raw := <-respCh:
		var r stream.ChartSelectResponse
		if err := json.Unmarshal(raw, &r); err != nil {
			return nil, err
		}
		if r.Cancelled {
			return nil, nil
		}
		if r.UseManualChart {
			if r.ManualRepoURL != "" && r.ManualChartName != "" {
				parts := strings.Split(strings.TrimRight(r.ManualRepoURL, "/"), "/")
				repoName := ""
				if len(parts) > 0 {
					repoName = parts[len(parts)-1]
				}
				return &catalog.ChartInfo{
					RepoName:  repoName,
					RepoURL:   r.ManualRepoURL,
					ChartName: r.ManualChartName,
					Source:    "manual",
				}, nil
			}
			return &catalog.ChartInfo{Source: "manual"}, nil
		}
		idx := r.CandidateIndex
		if idx >= 0 && idx < len(candidates) {
			c := candidates[idx]
			if c.Source == "" {
				c.Source = "artifacthub"
			}
			return &c, nil
		}
		return nil, fmt.Errorf("invalid chart selection index")
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-h.bridgeCtx.Done():
		return nil, h.bridgeCtx.Err()
	}
}

type streamDeployConfirmHandler struct {
	emitter   stream.Emitter
	queryID   string
	bridgeCtx context.Context
}

func (h *streamDeployConfirmHandler) ConfirmDeploy(ctx context.Context, plan deploytypes.DeployPlan) (deploytypes.DeployDecision, error) {
	payload, err := json.Marshal(plan)
	if err != nil {
		return deploytypes.DeployDecision{Action: "cancel"}, err
	}
	respCh := make(chan json.RawMessage, 1)
	ireq := stream.InteractionRequest{
		QueryID: h.queryID,
		Kind:    stream.KindDeployConfirm,
		Payload: payload,
		RespCh:  respCh,
	}
	if err := h.emitter.Emit(ctx, ireq); err != nil {
		return deploytypes.DeployDecision{Action: "cancel"}, err
	}
	select {
	case raw := <-respCh:
		var r stream.DeployConfirmResponse
		if err := json.Unmarshal(raw, &r); err != nil {
			return deploytypes.DeployDecision{Action: "cancel"}, err
		}
		return deploytypes.DeployDecision{
			Action:     r.Action,
			Values:     r.Values,
			Correction: r.Correction,
		}, nil
	case <-ctx.Done():
		return deploytypes.DeployDecision{Action: "cancel"}, ctx.Err()
	case <-h.bridgeCtx.Done():
		return deploytypes.DeployDecision{Action: "cancel"}, h.bridgeCtx.Err()
	}
}
