// pkg/agent/deploy/agent_test.go
package deploy

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kubewise/kubewise/pkg/catalog"
	"github.com/kubewise/kubewise/pkg/helm"
	"github.com/kubewise/kubewise/pkg/llm"
	"github.com/kubewise/kubewise/pkg/tool"
	"github.com/kubewise/kubewise/pkg/tui/events"
	"github.com/kubewise/kubewise/pkg/types"
)

// ---- mock implementations ----

type mockHelmClient struct {
	addRepoFunc            func(ctx context.Context, name, repoURL string) error
	fetchDefaultValuesFunc func(ctx context.Context, repoName, repoURL, chartName string) (string, error)
	installOrUpgradeFunc   func(ctx context.Context, opts helm.InstallOptions) (*helm.Release, error)
	statusFunc             func(ctx context.Context, releaseName, namespace string) (*helm.Release, error)
}

func (m *mockHelmClient) AddRepo(ctx context.Context, name, repoURL string) error {
	if m.addRepoFunc != nil {
		return m.addRepoFunc(ctx, name, repoURL)
	}
	return nil
}

func (m *mockHelmClient) FetchDefaultValues(ctx context.Context, repoName, repoURL, chartName string) (string, error) {
	if m.fetchDefaultValuesFunc != nil {
		return m.fetchDefaultValuesFunc(ctx, repoName, repoURL, chartName)
	}
	return "replicas: 1\nservice:\n  type: ClusterIP\n", nil
}

func (m *mockHelmClient) InstallOrUpgrade(ctx context.Context, opts helm.InstallOptions) (*helm.Release, error) {
	if m.installOrUpgradeFunc != nil {
		return m.installOrUpgradeFunc(ctx, opts)
	}
	return &helm.Release{Name: "nginx", Namespace: "default", Chart: "nginx-1.0.0", Status: "deployed"}, nil
}

func (m *mockHelmClient) Status(ctx context.Context, releaseName, namespace string) (*helm.Release, error) {
	if m.statusFunc != nil {
		return m.statusFunc(ctx, releaseName, namespace)
	}
	return nil, nil
}

type mockLLMClient struct {
	chatCompletionFunc func(ctx context.Context, messages []llm.Message, functions []llm.FunctionDefinition) (*llm.Message, error)
}

func (m *mockLLMClient) ChatCompletion(ctx context.Context, messages []llm.Message, functions []llm.FunctionDefinition) (*llm.Message, error) {
	if m.chatCompletionFunc != nil {
		return m.chatCompletionFunc(ctx, messages, functions)
	}
	return &llm.Message{Role: "assistant", Content: "replicas: 3"}, nil
}

type mockConfirmHandler struct {
	confirmDeployFunc func(ctx context.Context, plan events.DeployPlan) (events.DeployDecision, error)
}

func (m *mockConfirmHandler) ConfirmDeploy(ctx context.Context, plan events.DeployPlan) (events.DeployDecision, error) {
	if m.confirmDeployFunc != nil {
		return m.confirmDeployFunc(ctx, plan)
	}
	return events.DeployDecision{Action: "execute", Values: plan.CustomValues}, nil
}

type mockSelectionHandler struct {
	selectChartFunc func(ctx context.Context, appName string, candidates []catalog.ChartInfo) (*catalog.ChartInfo, error)
}

func (m *mockSelectionHandler) SelectChart(ctx context.Context, appName string, candidates []catalog.ChartInfo) (*catalog.ChartInfo, error) {
	if m.selectChartFunc != nil {
		return m.selectChartFunc(ctx, appName, candidates)
	}
	return &catalog.ChartInfo{
		RepoName:         "nginx-stable",
		RepoURL:          "https://helm.nginx.com/stable",
		ChartName:        "nginx",
		DefaultNamespace: "default",
		Description:      "NGINX Ingress Controller",
	}, nil
}

// ---- emit() unit tests ----

func TestEmit_SendsPhaseEvent(t *testing.T) {
	ch := make(chan events.TUIEvent, 10)
	agent := &Agent{eventCh: ch, queryID: "test-1"}

	agent.emit(events.PhaseEvent{QueryID: "test-1", Phase: "测试阶段"})

	select {
	case e := <-ch:
		pe, ok := e.(events.PhaseEvent)
		if !ok {
			t.Fatalf("expected PhaseEvent, got %T", e)
		}
		if pe.Phase != "测试阶段" {
			t.Errorf("expected phase '测试阶段', got %q", pe.Phase)
		}
		if pe.QueryID != "test-1" {
			t.Errorf("expected QueryID 'test-1', got %q", pe.QueryID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for PhaseEvent")
	}
}

func TestEmit_SendsToolCallEvent(t *testing.T) {
	ch := make(chan events.TUIEvent, 10)
	agent := &Agent{eventCh: ch, queryID: "test-1"}

	agent.emit(events.ToolCallEvent{QueryID: "test-1", ToolName: "helm repo add", Step: 1})

	select {
	case e := <-ch:
		tc, ok := e.(events.ToolCallEvent)
		if !ok {
			t.Fatalf("expected ToolCallEvent, got %T", e)
		}
		if tc.ToolName != "helm repo add" {
			t.Errorf("expected ToolName 'helm repo add', got %q", tc.ToolName)
		}
		if tc.Step != 1 {
			t.Errorf("expected Step 1, got %d", tc.Step)
		}
		if tc.QueryID != "test-1" {
			t.Errorf("expected QueryID 'test-1', got %q", tc.QueryID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for ToolCallEvent")
	}
}

func TestEmit_SendsToolDoneEvent(t *testing.T) {
	ch := make(chan events.TUIEvent, 10)
	agent := &Agent{eventCh: ch, queryID: "test-1"}

	agent.emit(events.ToolDoneEvent{QueryID: "test-1", ToolName: "helm repo add", Step: 1, Elapsed: time.Second})

	select {
	case e := <-ch:
		td, ok := e.(events.ToolDoneEvent)
		if !ok {
			t.Fatalf("expected ToolDoneEvent, got %T", e)
		}
		if td.ToolName != "helm repo add" {
			t.Errorf("expected ToolName 'helm repo add', got %q", td.ToolName)
		}
		if td.Step != 1 {
			t.Errorf("expected Step 1, got %d", td.Step)
		}
		if td.QueryID != "test-1" {
			t.Errorf("expected QueryID 'test-1', got %q", td.QueryID)
		}
		if td.Elapsed != time.Second {
			t.Errorf("expected Elapsed 1s, got %v", td.Elapsed)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for ToolDoneEvent")
	}
}

func TestEmit_NoChannel_NoPanic(t *testing.T) {
	agent := &Agent{}
	agent.emit(events.PhaseEvent{Phase: "测试"})
	agent.emit(events.ToolCallEvent{ToolName: "test", Step: 1})
	agent.emit(events.ToolDoneEvent{ToolName: "test", Step: 1, Elapsed: time.Second})
}

func TestEmit_FullChannel_NoBlock(t *testing.T) {
	ch := make(chan events.TUIEvent, 1)
	agent := &Agent{eventCh: ch, queryID: "test-1"}

	agent.emit(events.PhaseEvent{Phase: "first"})
	agent.emit(events.PhaseEvent{Phase: "second"})

	select {
	case e := <-ch:
		pe, ok := e.(events.PhaseEvent)
		if !ok {
			t.Fatalf("expected PhaseEvent, got %T", e)
		}
		if pe.Phase != "first" {
			t.Errorf("expected 'first', got %q", pe.Phase)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}

	select {
	case e := <-ch:
		t.Fatalf("expected no more events, got %T: %+v", e, e)
	default:
	}
}

// ---- integration test for HandleQuery event emission (standard flow) ----

func TestHandleQuery_EmitsAllPhaseAndToolEvents(t *testing.T) {
	ch := make(chan events.TUIEvent, 50)

	agent := &Agent{
		helmClient:       &mockHelmClient{},
		llmClient:        &mockLLMClient{},
		confirmHandler:   &mockConfirmHandler{},
		selectionHandler: &mockSelectionHandler{},
		eventCh:          ch,
		queryID:          "test-q-1",
	}

	result, err := agent.HandleQuery(context.Background(), "部署 nginx", types.Entities{AppName: "nginx"})
	if err != nil {
		t.Fatalf("HandleQuery failed: %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty result")
	}

	close(ch)

	var got []events.TUIEvent
	for e := range ch {
		got = append(got, e)
	}

	t.Logf("total events emitted: %d", len(got))
	for i, e := range got {
		t.Logf("  event[%d]: %T", i, e)
	}

	if len(got) < 20 {
		t.Fatalf("expected at least 20 events, got %d", len(got))
	}

	idx := 0

	// [0] AgentStartEvent: Deploy Agent
	as, ok := got[idx].(events.AgentStartEvent)
	if !ok {
		t.Fatalf("event[%d]: expected AgentStartEvent, got %T", idx, got[idx])
	}
	if as.AgentName != "Deploy Agent" {
		t.Errorf("event[%d]: expected AgentName 'Deploy Agent', got %q", idx, as.AgentName)
	}
	idx++

	// [1] PhaseEvent: 搜索 Chart: nginx
	pe, ok := got[idx].(events.PhaseEvent)
	if !ok {
		t.Fatalf("event[%d]: expected PhaseEvent, got %T", idx, got[idx])
	}
	if pe.Phase != "搜索 Chart: nginx" {
		t.Errorf("event[%d]: expected Phase '搜索 Chart: nginx', got %q", idx, pe.Phase)
	}
	idx++

	// [2] PhaseEvent: 获取 Chart 默认配置
	pe, ok = got[idx].(events.PhaseEvent)
	if !ok {
		t.Fatalf("event[%d]: expected PhaseEvent, got %T", idx, got[idx])
	}
	if pe.Phase != "获取 Chart 默认配置" {
		t.Errorf("event[%d]: expected Phase '获取 Chart 默认配置', got %q", idx, pe.Phase)
	}
	idx++

	// [3] ToolCallEvent: helm repo add Step 1
	tc, ok := got[idx].(events.ToolCallEvent)
	if !ok {
		t.Fatalf("event[%d]: expected ToolCallEvent, got %T", idx, got[idx])
	}
	if tc.ToolName != "helm repo add" {
		t.Errorf("event[%d]: expected ToolName 'helm repo add', got %q", idx, tc.ToolName)
	}
	if tc.Step != 1 {
		t.Errorf("event[%d]: expected Step 1, got %d", idx, tc.Step)
	}
	idx++

	// [4] ToolDoneEvent: helm repo add Step 1
	td, ok := got[idx].(events.ToolDoneEvent)
	if !ok {
		t.Fatalf("event[%d]: expected ToolDoneEvent, got %T", idx, got[idx])
	}
	if td.ToolName != "helm repo add" {
		t.Errorf("event[%d]: expected ToolName 'helm repo add', got %q", idx, td.ToolName)
	}
	if td.Step != 1 {
		t.Errorf("event[%d]: expected Step 1, got %d", idx, td.Step)
	}
	idx++

	// [5] ToolCallEvent: helm show values Step 2
	tc, ok = got[idx].(events.ToolCallEvent)
	if !ok {
		t.Fatalf("event[%d]: expected ToolCallEvent, got %T", idx, got[idx])
	}
	if tc.ToolName != "helm show values" {
		t.Errorf("event[%d]: expected ToolName 'helm show values', got %q", idx, tc.ToolName)
	}
	if tc.Step != 2 {
		t.Errorf("event[%d]: expected Step 2, got %d", idx, tc.Step)
	}
	idx++

	// [6] ToolDoneEvent: helm show values Step 2
	td, ok = got[idx].(events.ToolDoneEvent)
	if !ok {
		t.Fatalf("event[%d]: expected ToolDoneEvent, got %T", idx, got[idx])
	}
	if td.ToolName != "helm show values" {
		t.Errorf("event[%d]: expected ToolName 'helm show values', got %q", idx, td.ToolName)
	}
	if td.Step != 2 {
		t.Errorf("event[%d]: expected Step 2, got %d", idx, td.Step)
	}
	idx++

	// [7] PhaseEvent: 生成配置建议
	pe, ok = got[idx].(events.PhaseEvent)
	if !ok {
		t.Fatalf("event[%d]: expected PhaseEvent, got %T", idx, got[idx])
	}
	if pe.Phase != "生成配置建议" {
		t.Errorf("event[%d]: expected Phase '生成配置建议', got %q", idx, pe.Phase)
	}
	idx++

	// [8] ToolCallEvent: LLM values generation Step 3
	tc, ok = got[idx].(events.ToolCallEvent)
	if !ok {
		t.Fatalf("event[%d]: expected ToolCallEvent, got %T", idx, got[idx])
	}
	if tc.ToolName != "LLM values generation" {
		t.Errorf("event[%d]: expected ToolName 'LLM values generation', got %q", idx, tc.ToolName)
	}
	if tc.Step != 3 {
		t.Errorf("event[%d]: expected Step 3, got %d", idx, tc.Step)
	}
	idx++

	// [9] ToolDoneEvent: LLM values generation Step 3
	td, ok = got[idx].(events.ToolDoneEvent)
	if !ok {
		t.Fatalf("event[%d]: expected ToolDoneEvent, got %T", idx, got[idx])
	}
	if td.ToolName != "LLM values generation" {
		t.Errorf("event[%d]: expected ToolName 'LLM values generation', got %q", idx, td.ToolName)
	}
	if td.Step != 3 {
		t.Errorf("event[%d]: expected Step 3, got %d", idx, td.Step)
	}
	idx++

	// [10] PhaseEvent: 等待用户确认
	pe, ok = got[idx].(events.PhaseEvent)
	if !ok {
		t.Fatalf("event[%d]: expected PhaseEvent, got %T", idx, got[idx])
	}
	if pe.Phase != "等待用户确认" {
		t.Errorf("event[%d]: expected Phase '等待用户确认', got %q", idx, pe.Phase)
	}
	idx++

	// [11] ToolCallEvent: user confirm Step 4
	tc, ok = got[idx].(events.ToolCallEvent)
	if !ok {
		t.Fatalf("event[%d]: expected ToolCallEvent, got %T", idx, got[idx])
	}
	if tc.ToolName != "user confirm" {
		t.Errorf("event[%d]: expected ToolName 'user confirm', got %q", idx, tc.ToolName)
	}
	if tc.Step != 4 {
		t.Errorf("event[%d]: expected Step 4, got %d", idx, tc.Step)
	}
	idx++

	// [12] ToolDoneEvent: user confirm Step 4
	td, ok = got[idx].(events.ToolDoneEvent)
	if !ok {
		t.Fatalf("event[%d]: expected ToolDoneEvent, got %T", idx, got[idx])
	}
	if td.ToolName != "user confirm" {
		t.Errorf("event[%d]: expected ToolName 'user confirm', got %q", idx, td.ToolName)
	}
	if td.Step != 4 {
		t.Errorf("event[%d]: expected Step 4, got %d", idx, td.Step)
	}
	idx++

	// [13] PhaseEvent: 执行部署
	pe, ok = got[idx].(events.PhaseEvent)
	if !ok {
		t.Fatalf("event[%d]: expected PhaseEvent, got %T", idx, got[idx])
	}
	if pe.Phase != "执行部署" {
		t.Errorf("event[%d]: expected Phase '执行部署', got %q", idx, pe.Phase)
	}
	idx++

	// [14] ToolCallEvent: helm install/upgrade Step 6
	tc, ok = got[idx].(events.ToolCallEvent)
	if !ok {
		t.Fatalf("event[%d]: expected ToolCallEvent, got %T", idx, got[idx])
	}
	if tc.ToolName != "helm install/upgrade" {
		t.Errorf("event[%d]: expected ToolName 'helm install/upgrade', got %q", idx, tc.ToolName)
	}
	if tc.Step != 6 {
		t.Errorf("event[%d]: expected Step 6, got %d", idx, tc.Step)
	}
	idx++

	// [15] ToolDoneEvent: helm install/upgrade Step 6
	td, ok = got[idx].(events.ToolDoneEvent)
	if !ok {
		t.Fatalf("event[%d]: expected ToolDoneEvent, got %T", idx, got[idx])
	}
	if td.ToolName != "helm install/upgrade" {
		t.Errorf("event[%d]: expected ToolName 'helm install/upgrade', got %q", idx, td.ToolName)
	}
	if td.Step != 6 {
		t.Errorf("event[%d]: expected Step 6, got %d", idx, td.Step)
	}
	idx++

	// [16] PhaseEvent: 验证部署状态
	pe, ok = got[idx].(events.PhaseEvent)
	if !ok {
		t.Fatalf("event[%d]: expected PhaseEvent, got %T", idx, got[idx])
	}
	if pe.Phase != "验证部署状态" {
		t.Errorf("event[%d]: expected Phase '验证部署状态', got %q", idx, pe.Phase)
	}
	idx++

	// [17] ToolCallEvent: verify deployment Step 7
	tc, ok = got[idx].(events.ToolCallEvent)
	if !ok {
		t.Fatalf("event[%d]: expected ToolCallEvent, got %T", idx, got[idx])
	}
	if tc.ToolName != "verify deployment" {
		t.Errorf("event[%d]: expected ToolName 'verify deployment', got %q", idx, tc.ToolName)
	}
	if tc.Step != 7 {
		t.Errorf("event[%d]: expected Step 7, got %d", idx, tc.Step)
	}
	idx++

	// [18] ToolDoneEvent: verify deployment Step 7
	td, ok = got[idx].(events.ToolDoneEvent)
	if !ok {
		t.Fatalf("event[%d]: expected ToolDoneEvent, got %T", idx, got[idx])
	}
	if td.ToolName != "verify deployment" {
		t.Errorf("event[%d]: expected ToolName 'verify deployment', got %q", idx, td.ToolName)
	}
	if td.Step != 7 {
		t.Errorf("event[%d]: expected Step 7, got %d", idx, td.Step)
	}
}

// ---- integration tests for NL correction path ----

func TestHandleQuery_WithNLCorrection_EmitsStep5Events(t *testing.T) {
	ch := make(chan events.TUIEvent, 50)
	confirmCallCount := 0

	agent := &Agent{
		helmClient: &mockHelmClient{},
		llmClient:  &mockLLMClient{},
		selectionHandler: &mockSelectionHandler{},
		confirmHandler: &mockConfirmHandler{
			confirmDeployFunc: func(ctx context.Context, plan events.DeployPlan) (events.DeployDecision, error) {
				confirmCallCount++
				if confirmCallCount == 1 {
					return events.DeployDecision{
						Action:     "execute",
						Values:     "replicas: 1",
						Correction: "increase replicas to 5",
					}, nil
				}
				return events.DeployDecision{Action: "execute", Values: "replicas: 5"}, nil
			},
		},
		eventCh: ch,
		queryID: "test-nl-q",
	}

	result, err := agent.HandleQuery(context.Background(), "部署 nginx", types.Entities{AppName: "nginx"})
	if err != nil {
		t.Fatalf("HandleQuery failed: %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty result")
	}

	close(ch)

	var got []events.TUIEvent
	for e := range ch {
		got = append(got, e)
	}

	t.Logf("total events emitted with NL correction: %d", len(got))
	for i, e := range got {
		t.Logf("  event[%d]: %T %+v", i, e, e)
	}

	if len(got) < 24 {
		t.Fatalf("expected at least 24 events with NL correction, got %d", len(got))
	}

	idx := 13

	// [13] ToolCallEvent: LLM values regeneration Step 5
	tc, ok := got[idx].(events.ToolCallEvent)
	if !ok {
		t.Fatalf("event[%d]: expected ToolCallEvent, got %T", idx, got[idx])
	}
	if tc.ToolName != "LLM values regeneration" {
		t.Errorf("event[%d]: expected ToolName 'LLM values regeneration', got %q", idx, tc.ToolName)
	}
	if tc.Step != 5 {
		t.Errorf("event[%d]: expected Step 5, got %d", idx, tc.Step)
	}
	idx++

	// [14] ToolDoneEvent: LLM values regeneration Step 5
	td, ok := got[idx].(events.ToolDoneEvent)
	if !ok {
		t.Fatalf("event[%d]: expected ToolDoneEvent, got %T", idx, got[idx])
	}
	if td.ToolName != "LLM values regeneration" {
		t.Errorf("event[%d]: expected ToolName 'LLM values regeneration', got %q", idx, td.ToolName)
	}
	if td.Step != 5 {
		t.Errorf("event[%d]: expected Step 5, got %d", idx, td.Step)
	}
	idx++

	// [15] ToolCallEvent: user confirm (second) Step 4
	tc, ok = got[idx].(events.ToolCallEvent)
	if !ok {
		t.Fatalf("event[%d]: expected ToolCallEvent, got %T", idx, got[idx])
	}
	if tc.ToolName != "user confirm" {
		t.Errorf("event[%d]: expected ToolName 'user confirm', got %q", idx, tc.ToolName)
	}
	if tc.Step != 4 {
		t.Errorf("event[%d]: expected Step 4, got %d", idx, tc.Step)
	}
	idx++

	// [16] ToolDoneEvent: user confirm (second) Step 4
	td, ok = got[idx].(events.ToolDoneEvent)
	if !ok {
		t.Fatalf("event[%d]: expected ToolDoneEvent, got %T", idx, got[idx])
	}
	if td.ToolName != "user confirm" {
		t.Errorf("event[%d]: expected ToolName 'user confirm', got %q", idx, td.ToolName)
	}
	if td.Step != 4 {
		t.Errorf("event[%d]: expected Step 4, got %d", idx, td.Step)
	}
}

func TestHandleQuery_WithNLCorrection_CancelAfterRegen(t *testing.T) {
	ch := make(chan events.TUIEvent, 50)
	confirmCallCount := 0

	agent := &Agent{
		helmClient: &mockHelmClient{},
		llmClient:  &mockLLMClient{},
		selectionHandler: &mockSelectionHandler{},
		confirmHandler: &mockConfirmHandler{
			confirmDeployFunc: func(ctx context.Context, plan events.DeployPlan) (events.DeployDecision, error) {
				confirmCallCount++
				if confirmCallCount == 1 {
					return events.DeployDecision{
						Action:     "execute",
						Values:     "replicas: 1",
						Correction: "increase replicas to 5",
					}, nil
				}
				return events.DeployDecision{Action: "cancel"}, nil
			},
		},
		eventCh: ch,
		queryID: "test-nl-cancel",
	}

	result, err := agent.HandleQuery(context.Background(), "部署 nginx", types.Entities{AppName: "nginx"})
	if err != nil {
		t.Fatalf("HandleQuery failed: %v", err)
	}
	if result != "部署已取消" {
		t.Errorf("expected '部署已取消', got %q", result)
	}

	close(ch)

	var got []events.TUIEvent
	for e := range ch {
		got = append(got, e)
	}

	t.Logf("total events emitted (cancel after regen): %d", len(got))

	hasStep5ToolCall := false
	hasStep5ToolDone := false
	for _, e := range got {
		if tc, ok := e.(events.ToolCallEvent); ok && tc.ToolName == "LLM values regeneration" && tc.Step == 5 {
			hasStep5ToolCall = true
		}
		if td, ok := e.(events.ToolDoneEvent); ok && td.ToolName == "LLM values regeneration" && td.Step == 5 {
			hasStep5ToolDone = true
		}
	}

	if !hasStep5ToolCall {
		t.Error("expected ToolCallEvent for LLM values regeneration (Step 5)")
	}
	if !hasStep5ToolDone {
		t.Error("expected ToolDoneEvent for LLM values regeneration (Step 5)")
	}

	hasPhaseDeploy := false
	for _, e := range got {
		if pe, ok := e.(events.PhaseEvent); ok && pe.Phase == "执行部署" {
			hasPhaseDeploy = true
		}
	}
	if hasPhaseDeploy {
		t.Error("should NOT emit '执行部署' PhaseEvent after cancel")
	}

	_ = confirmCallCount
}

// ---- recoverDeploy tests ----

func TestRecoverDeploy_SubmitReport(t *testing.T) {
	llmClient := &mockLLMClient{
		chatCompletionFunc: func(ctx context.Context, messages []llm.Message, functions []llm.FunctionDefinition) (*llm.Message, error) {
			return &llm.Message{
				Role: "assistant",
				ToolCalls: []llm.ToolCall{{
					ID:   "call_1",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "submit_report",
						Arguments: map[string]any{"reason": "镜像拉取失败", "details": "检查发现镜像标签 latest 不存在"},
					},
				}},
			}, nil
		},
	}
	agent := &Agent{
		llmClient:    llmClient,
		helmClient:   &mockHelmClient{},
		toolRegistry: tool.NewRegistry(tool.ToolDependency{}),
	}

	result, err := agent.recoverDeploy(context.Background(),
		fmt.Errorf("install timed out"),
			"", nil,
		&catalog.ChartInfo{ChartName: "nginx", RepoName: "nginx", RepoURL: "https://helm.nginx.com/stable"},
		"replicas: 1\n", "replicas: 3\n", "default", "nginx",
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Action != ActionAbort {
		t.Fatalf("expected ActionAbort, got %v", result.Action)
	}
	if result.Reason != "镜像拉取失败" {
		t.Fatalf("expected reason '镜像拉取失败', got %q", result.Reason)
	}
	if !strings.Contains(result.Details, "镜像标签") {
		t.Fatalf("expected details about image tag, got %q", result.Details)
	}
}

func TestRecoverDeploy_NilRegistry(t *testing.T) {
	agent := &Agent{}
	result, err := agent.recoverDeploy(context.Background(),
		fmt.Errorf("install timed out"),
			"", nil,
			&catalog.ChartInfo{ChartName: "nginx"},
		"", "", "default", "nginx",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Action != ActionAbort {
		t.Fatalf("expected ActionAbort, got %v", result.Action)
	}
	if !strings.Contains(result.Reason, "诊断工具不可用") {
		t.Fatalf("expected reason about unavailable tools, got %q", result.Reason)
	}
}

func TestRecoverDeploy_StepLimit(t *testing.T) {
	llmClient := &mockLLMClient{
		chatCompletionFunc: func(ctx context.Context, messages []llm.Message, functions []llm.FunctionDefinition) (*llm.Message, error) {
			return &llm.Message{Role: "assistant", Content: "still thinking..."}, nil
		},
	}
	agent := &Agent{
		llmClient:    llmClient,
		helmClient:   &mockHelmClient{},
		toolRegistry: tool.NewRegistry(tool.ToolDependency{}),
	}

	result, err := agent.recoverDeploy(context.Background(),
		fmt.Errorf("install timed out"),
			"", nil,
			&catalog.ChartInfo{ChartName: "nginx"},
			"", "", "default", "nginx",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Action != ActionAbort {
		t.Fatalf("expected ActionAbort, got %v", result.Action)
	}
	if !strings.Contains(result.Reason, "最大步数") {
		t.Fatalf("expected reason about step limit, got %q", result.Reason)
	}
}
