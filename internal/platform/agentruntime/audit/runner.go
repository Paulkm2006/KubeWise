package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kubewise/kubewise/internal/audit/domain"
	"github.com/kubewise/kubewise/internal/platform/agentruntime"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/event"
	securitytools "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/security"
	toolv2 "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/v2"
	"github.com/kubewise/kubewise/internal/platform/cluster"
)

type phaseDef struct {
	ID       string
	Label    string
	ToolName string
	Category string
}

var auditPhases = []phaseDef{
	{ID: "rbac", Label: "RBAC", ToolName: securitytools.AuditRBACToolName, Category: "RBAC"},
	{ID: "pod_security", Label: "Pod Security", ToolName: securitytools.AuditPodSecurityToolName, Category: "Pod Security"},
	{ID: "network_policies", Label: "Network Policies", ToolName: securitytools.AuditNetworkPoliciesToolName, Category: "Network Policy"},
	{ID: "image_security", Label: "Image Security", ToolName: securitytools.AuditImageSecurityToolName, Category: "Image Security"},
}

type Runner struct {
	k8s *cluster.Client
}

func NewRunner(k8s *cluster.Client) *Runner {
	return &Runner{k8s: k8s}
}

func (r *Runner) Run(ctx context.Context, clusterName, queryID string, out chan<- agentruntime.ProgressEvent) error {
	start := time.Now()
	reg := toolv2.NewRegistry()
	if err := securitytools.RegisterAuditTools(reg, r.k8s); err != nil {
		return fmt.Errorf("register audit tools: %w", err)
	}
	emit := func(pe agentruntime.ProgressEvent) {
		select {
		case out <- pe:
		case <-ctx.Done():
		}
	}

	var allFindings []domain.Finding
	for i, phase := range auditPhases {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		phaseStart := time.Now()
		emit(agentruntime.ProgressEvent{
			Type:    "phase_start",
			Message: phase.ID,
			Summary: fmt.Sprintf("Scanning %s…", phase.Label),
		})

		tool, ok := reg.Get(phase.ToolName)
		if !ok {
			return fmt.Errorf("audit tool %q not registered", phase.ToolName)
		}
		result, err := tool.Execute(ctx, map[string]any{})
		if err != nil {
			emit(agentruntime.ProgressEvent{
				Type: "phase_fail", Message: phase.ID, Detail: err.Error(),
			})
			return fmt.Errorf("%s: %w", phase.Label, err)
		}

		text := result.Display
		findings := ParseFindings(phase.Category, text)
		allFindings = append(allFindings, findings...)

		payload, _ := json.Marshal(map[string]any{
			"phase":     phase.ID,
			"label":     phase.Label,
			"index":     i + 1,
			"total":     len(auditPhases),
			"count":     len(findings),
			"findings":  findings,
			"elapsed_ms": time.Since(phaseStart).Milliseconds(),
		})
		emit(agentruntime.ProgressEvent{
			Type:        "phase_done",
			Message:     phase.ID,
			Summary:     fmt.Sprintf("%s complete — %d findings", phase.Label, len(findings)),
			PayloadKind: event.PayloadKindAuditPhaseFindings,
			PayloadJSON: string(payload),
			ElapsedMs:   int(time.Since(phaseStart).Milliseconds()),
		})
	}

	durationMs := time.Since(start).Milliseconds()
	summary := Summarize(allFindings)
	markdown := RenderMarkdown(clusterName, allFindings, summary, start, durationMs)
	report := domain.Result{
		Findings: allFindings, Summary: summary, Markdown: markdown, DurationMs: durationMs,
	}
	reportJSON, _ := json.Marshal(report)
	emit(agentruntime.ProgressEvent{
		Type:        "audit_complete",
		Summary:     fmt.Sprintf("Audit complete — %d findings", summary.Total),
		PayloadKind: event.PayloadKindAuditReport,
		PayloadJSON: string(reportJSON),
		ElapsedMs:   int(durationMs),
	})
	return nil
}
