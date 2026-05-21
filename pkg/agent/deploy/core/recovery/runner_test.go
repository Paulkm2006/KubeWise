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
	runner := &Runner{QueryID: "q-final-report"}

	_, done, result, err := runner.handleToolCall(
		context.Background(),
		llm.ToolCall{ID: "report", Function: llm.FunctionCall{
			Name: "submit_report", Arguments: map[string]any{"reason": "failed", "details": "final deploy failure"},
		}},
		&catalog.ChartInfo{ChartName: "nginx"}, "", ptr(""), "default", "nginx",
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !done || result == nil || result.Details != "final deploy failure" {
		t.Fatalf("expected final result, done=%v result=%+v", done, result)
	}
}

func TestRunner_SubmitValuesReturnsCandidateWithoutSideEffects(t *testing.T) {
	installCalled := false
	runner := &Runner{
		Helm: &mockHelm{
			installOrUpgradeFunc: func(ctx context.Context, opts helm.InstallOptions) (*helm.Release, error) {
				installCalled = true
				return nil, fmt.Errorf("must not install")
			},
		},
	}

	_, done, result, err := runner.handleToolCall(
		context.Background(),
		llm.ToolCall{ID: "values", Function: llm.FunctionCall{
			Name: "submit_values", Arguments: map[string]any{"yaml": "replicas: 2", "summary": "reduce replicas"},
		}},
		&catalog.ChartInfo{ChartName: "nginx"}, "replicas: 1\n", ptr("replicas: 3\n"), "default", "nginx",
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !done || result == nil || result.Action != ActionRecovered {
		t.Fatalf("expected recovered values candidate, done=%v result=%+v", done, result)
	}
	if result.YAML != "replicas: 2" || result.Summary != "reduce replicas" {
		t.Fatalf("unexpected candidate result: %+v", result)
	}
	if installCalled {
		t.Fatal("recovery core must not install or upgrade releases")
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
