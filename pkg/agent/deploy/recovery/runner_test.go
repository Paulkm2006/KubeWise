package recovery

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/kubewise/kubewise/pkg/agent/troubleshooting"
	"github.com/kubewise/kubewise/pkg/catalog"
	"github.com/kubewise/kubewise/pkg/helm"
	"github.com/kubewise/kubewise/pkg/llm"
	"github.com/kubewise/kubewise/pkg/tool"
	"github.com/kubewise/kubewise/pkg/tui/events"
)

type mockLLM struct {
	chat func(ctx context.Context, messages []llm.Message, functions []llm.FunctionDefinition) (*llm.Message, error)
}

func (m *mockLLM) ChatCompletion(ctx context.Context, messages []llm.Message, functions []llm.FunctionDefinition) (*llm.Message, error) {
	return m.chat(ctx, messages, functions)
}

type mockHelm struct {
	statusFunc           func(ctx context.Context, releaseName, namespace string) (*helm.Release, error)
	validateFunc         func(ctx context.Context, opts helm.RenderOptions) (*helm.ValidationResult, error)
	installOrUpgradeFunc func(ctx context.Context, opts helm.InstallOptions) (*helm.Release, error)
}

func (m *mockHelm) AddRepo(ctx context.Context, name, repoURL string) error { return nil }
func (m *mockHelm) FetchDefaultValues(ctx context.Context, repoName, repoURL, chartName string) (string, error) {
	return "", nil
}
func (m *mockHelm) Status(ctx context.Context, releaseName, namespace string) (*helm.Release, error) {
	if m.statusFunc != nil {
		return m.statusFunc(ctx, releaseName, namespace)
	}
	return nil, fmt.Errorf("not found")
}
func (m *mockHelm) Validate(ctx context.Context, opts helm.RenderOptions) (*helm.ValidationResult, error) {
	if m.validateFunc != nil {
		return m.validateFunc(ctx, opts)
	}
	return &helm.ValidationResult{}, nil
}
func (m *mockHelm) InstallOrUpgrade(ctx context.Context, opts helm.InstallOptions) (*helm.Release, error) {
	if m.installOrUpgradeFunc != nil {
		return m.installOrUpgradeFunc(ctx, opts)
	}
	return &helm.Release{Name: opts.ReleaseName, Namespace: opts.Namespace, Status: "deployed"}, nil
}

func TestRunner_MultipleToolCalls_AllHandled(t *testing.T) {
	called := 0
	runner := &Runner{
		LLM: &mockLLM{chat: func(ctx context.Context, messages []llm.Message, functions []llm.FunctionDefinition) (*llm.Message, error) {
			called++
			if called == 1 {
				return &llm.Message{
					ToolCalls: []llm.ToolCall{
						{ID: "a", Type: "function", Function: llm.FunctionCall{Name: "unknown_tool_x", Arguments: map[string]any{}}},
						{ID: "b", Type: "function", Function: llm.FunctionCall{
							Name: "submit_report", Arguments: map[string]any{"reason": "done", "details": "checked"},
						}},
					},
				}, nil
			}
			return &llm.Message{Role: "assistant", Content: "done"}, nil
		}},
		Helm:  &mockHelm{},
		Tools: tool.NewRegistry(tool.ToolDependency{}),
	}
	result, err := runner.Run(context.Background(), RunInput{
		DeployErr: fmt.Errorf("fail"), Chart: &catalog.ChartInfo{ChartName: "nginx"},
		TargetNS: "default", AppName: "nginx",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != "done" {
		t.Fatalf("expected submit_report result, got %q", result.Reason)
	}
}

func TestRunner_SubmitReportReturnsFinalDetailsWithoutEmitting(t *testing.T) {
	ch := make(chan events.TUIEvent, 10)
	runner := &Runner{QueryID: "q-final-report", EmitCritical: func(e events.TUIEvent) { ch <- e }}

	_, done, result, err := runner.handleToolCall(
		context.Background(),
		llm.ToolCall{ID: "report", Function: llm.FunctionCall{
			Name: "submit_report", Arguments: map[string]any{"reason": "failed", "details": "final deploy failure"},
		}},
		&catalog.ChartInfo{ChartName: "nginx"}, "", ptr(""), "default", "nginx", ptrInt(0),
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !done || result == nil || result.Details != "final deploy failure" {
		t.Fatalf("expected final result, done=%v result=%+v", done, result)
	}
	select {
	case e := <-ch:
		t.Fatalf("final report should be returned, not emitted directly: %T %+v", e, e)
	default:
	}
}

func TestContext_TruncatesToolResults(t *testing.T) {
	rc := NewContext([]llm.Message{
		{Role: "system", Content: "system"},
		{Role: "user", Content: "user"},
	}, 16)

	rc.AppendAssistant(llm.Message{Role: "assistant", Content: "call", ToolCalls: []llm.ToolCall{{ID: "tool-1"}}})
	rc.AppendToolResult("tool-1", strings.Repeat("x", 64))

	messages := rc.Messages()
	if len(messages) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(messages))
	}
	toolMsg := messages[3]
	if toolMsg.Role != "tool" || toolMsg.ToolCallID != "tool-1" {
		t.Fatalf("expected tool message, got %+v", toolMsg)
	}
	if len(toolMsg.Content) >= 64 {
		t.Fatalf("expected truncated tool result, got %d bytes", len(toolMsg.Content))
	}
	if !strings.Contains(toolMsg.Content, "输出已截断") {
		t.Fatalf("expected truncation marker, got %q", toolMsg.Content)
	}
}

func TestRunner_DeployAttemptLimit(t *testing.T) {
	installCount := 0
	runner := &Runner{
		LLM: &mockLLM{chat: func(ctx context.Context, messages []llm.Message, functions []llm.FunctionDefinition) (*llm.Message, error) {
			return &llm.Message{
				ToolCalls: []llm.ToolCall{{
					ID: "v1", Type: "function",
					Function: llm.FunctionCall{
						Name: "submit_values", Arguments: map[string]any{"yaml": "replicas: 1", "summary": "retry"},
					},
				}},
			}, nil
		}},
		Helm: &mockHelm{
			installOrUpgradeFunc: func(ctx context.Context, opts helm.InstallOptions) (*helm.Release, error) {
				installCount++
				return nil, fmt.Errorf("still failing")
			},
		},
		Confirm: func(ctx context.Context, plan events.DeployPlan) (events.DeployDecision, error) {
			return events.DeployDecision{Action: "execute", Values: plan.CustomValues}, nil
		},
		Tools: tool.NewRegistry(tool.ToolDependency{}),
	}

	submit := func(attempts *int) string {
		msg, _, _, err := runner.handleSubmitValues(
			context.Background(),
			llm.ToolCall{ID: "x", Function: llm.FunctionCall{
				Name: "submit_values", Arguments: map[string]any{"yaml": "replicas: 1", "summary": "s"},
			}},
			&catalog.ChartInfo{ChartName: "nginx", RepoName: "r", RepoURL: "https://x"},
			"replicas: 1\n", ptr("replicas: 1\n"), "default", "nginx", attempts,
		)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		return msg
	}

	attempts := 0
	submit(&attempts)
	submit(&attempts)

	if installCount != maxRecoverDeployAttempts {
		t.Fatalf("expected %d install attempts, got %d", maxRecoverDeployAttempts, installCount)
	}

	msg := submit(&attempts)
	if !strings.Contains(msg, "最大重新部署次数") {
		t.Fatalf("expected limit message, got %q", msg)
	}
}

func TestRunner_SubmitValues_UsesInstallPreflightWhenReleaseAbsent(t *testing.T) {
	var gotIsUpgrade bool
	runner := &Runner{
		Helm: &mockHelm{
			statusFunc: func(ctx context.Context, releaseName, namespace string) (*helm.Release, error) {
				return nil, fmt.Errorf("not found")
			},
			validateFunc: func(ctx context.Context, opts helm.RenderOptions) (*helm.ValidationResult, error) {
				gotIsUpgrade = opts.IsUpgrade
				return &helm.ValidationResult{}, nil
			},
		},
		Confirm: func(ctx context.Context, plan events.DeployPlan) (events.DeployDecision, error) {
			return events.DeployDecision{Action: "execute", Values: plan.CustomValues}, nil
		},
	}
	attempts := 0

	_, _, _, err := runner.handleSubmitValues(
		context.Background(),
		llm.ToolCall{ID: "x", Function: llm.FunctionCall{
			Name: "submit_values", Arguments: map[string]any{"yaml": "replicas: 1", "summary": "retry"},
		}},
		&catalog.ChartInfo{ChartName: "nginx", RepoName: "r", RepoURL: "https://x"},
		"replicas: 1\n", ptr("replicas: 1\n"), "default", "nginx", &attempts,
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if gotIsUpgrade {
		t.Fatal("expected recovery preflight to use install semantics when release is absent")
	}
}

func TestRunner_SubmitValues_RetriesPreflightAsInstallWhenNoDeployedRelease(t *testing.T) {
	validateCalls := 0
	installCalls := 0
	runner := &Runner{
		Helm: &mockHelm{
			statusFunc: func(ctx context.Context, releaseName, namespace string) (*helm.Release, error) {
				return &helm.Release{Name: releaseName, Namespace: namespace, Status: "deployed"}, nil
			},
			validateFunc: func(ctx context.Context, opts helm.RenderOptions) (*helm.ValidationResult, error) {
				validateCalls++
				if opts.IsUpgrade {
					return &helm.ValidationResult{RenderErr: fmt.Errorf(`helm render 失败: "nginx" has no deployed releases`)}, nil
				}
				return &helm.ValidationResult{}, nil
			},
			installOrUpgradeFunc: func(ctx context.Context, opts helm.InstallOptions) (*helm.Release, error) {
				installCalls++
				return &helm.Release{Name: opts.ReleaseName, Namespace: opts.Namespace, Status: "deployed"}, nil
			},
		},
		Confirm: func(ctx context.Context, plan events.DeployPlan) (events.DeployDecision, error) {
			return events.DeployDecision{Action: "execute", Values: plan.CustomValues}, nil
		},
		BuildReport: func(ctx context.Context, rel *helm.Release, chart *catalog.ChartInfo, namespace, releaseName string) string {
			return "ok"
		},
	}
	attempts := 0

	_, done, result, err := runner.handleSubmitValues(
		context.Background(),
		llm.ToolCall{ID: "x", Function: llm.FunctionCall{
			Name: "submit_values", Arguments: map[string]any{"yaml": "replicas: 1", "summary": "retry"},
		}},
		&catalog.ChartInfo{ChartName: "nginx", RepoName: "r", RepoURL: "https://x"},
		"replicas: 1\n", ptr("replicas: 1\n"), "default", "nginx", &attempts,
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !done || result == nil || result.Action != ActionRecovered {
		t.Fatalf("expected successful recovery after install preflight retry, done=%v result=%+v", done, result)
	}
	if validateCalls != 2 {
		t.Fatalf("expected upgrade preflight followed by install preflight, got %d calls", validateCalls)
	}
	if installCalls != 1 {
		t.Fatalf("expected one redeploy after preflight retry, got %d", installCalls)
	}
}

func TestRecoveryToolDefs_ExcludeAuditPrefixes(t *testing.T) {
	reg, err := tool.LoadGlobalRegistryByCategory(tool.ToolDependency{}, "")
	if err != nil {
		t.Fatal(err)
	}
	defs := troubleshooting.RecoveryToolDefinitions(reg)
	for _, d := range defs {
		if strings.HasPrefix(d.Name, "audit_") {
			t.Fatalf("audit tool %q should not be offered to recovery", d.Name)
		}
	}
}

func ptr(s string) *string { return &s }

func ptrInt(i int) *int { return &i }
