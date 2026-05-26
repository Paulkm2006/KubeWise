package values

import (
	"context"
	"testing"

	"github.com/kubewise/kubewise/pkg/catalog"
	"github.com/kubewise/kubewise/pkg/llm"
)

type mockLLMClient struct {
	chatCompletionFunc func(ctx context.Context, messages []llm.Message, functions []llm.FunctionDefinition) (*llm.Message, error)
}

func (m *mockLLMClient) ChatCompletion(ctx context.Context, messages []llm.Message, functions []llm.FunctionDefinition, onChunk func(llm.StreamChunk)) (*llm.Message, error) {
	return m.chatCompletionFunc(ctx, messages, functions)
}

func TestParseGeneratedValuesArgs(t *testing.T) {
	chart := &catalog.ChartInfo{ChartName: "redis", DefaultNamespace: ""}
	result, err := parseGeneratedValuesArgs(map[string]any{
		"namespace":   "default",
		"values_yaml": "replicas: 2",
		"explanation": "scale up",
		"risk_level":  "low",
	}, chart)
	if err != nil {
		t.Fatal(err)
	}
	if result.Values != "replicas: 2" || result.Namespace != "default" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestParseGeneratedValuesArgs_BlocksKubeSystem(t *testing.T) {
	chart := &catalog.ChartInfo{ChartName: "myapp"}
	result, err := parseGeneratedValuesArgs(map[string]any{
		"namespace":   "kube-system",
		"values_yaml": "",
		"explanation": "bad",
		"risk_level":  "low",
	}, chart)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateGeneratedValues(result); err == nil {
		t.Fatal("expected namespace validation error")
	}
}

func TestResolveNamespace_LLMPrefersUserChoice(t *testing.T) {
	chart := &catalog.ChartInfo{ChartName: "prometheus", DefaultNamespace: "monitoring"}
	ns := resolveNamespace(chart, "dev")
	if ns != "dev" {
		t.Fatalf("expected user/LLM namespace dev, got %q", ns)
	}
}

func TestResolveNamespace_FallbackWhenEmpty(t *testing.T) {
	chart := &catalog.ChartInfo{ChartName: "prometheus", DefaultNamespace: ""}
	ns := resolveNamespace(chart, "")
	if ns != "monitoring" {
		t.Fatalf("expected convention hint monitoring when empty, got %q", ns)
	}
}

func TestResolveNamespace_ChartDefaultWhenEmpty(t *testing.T) {
	chart := &catalog.ChartInfo{ChartName: "argo-cd", DefaultNamespace: "argocd"}
	ns := resolveNamespace(chart, "")
	if ns != "argocd" {
		t.Fatalf("expected chart default argocd, got %q", ns)
	}
}

func TestGenerate_FunctionCalling(t *testing.T) {
	mock := &mockLLMClient{
		chatCompletionFunc: func(ctx context.Context, messages []llm.Message, functions []llm.FunctionDefinition) (*llm.Message, error) {
			if len(functions) == 0 || functions[0].Name != SubmitGeneratedValuesFnName {
				t.Fatalf("expected submit_generated_values in tools, got %+v", functions)
			}
			return &llm.Message{
				ToolCalls: []llm.ToolCall{{
					ID: "c1", Type: "function",
					Function: llm.FunctionCall{
						Name: SubmitGeneratedValuesFnName,
						Arguments: map[string]any{
							"namespace":   "staging",
							"values_yaml": "replicas: 3",
							"explanation": "increase replicas",
							"risk_level":  "low",
						},
					},
				}},
			}, nil
		},
	}
	result, err := callValuesLLM(context.Background(), mock, valuesGenSystemPrompt, "prompt",
		&catalog.ChartInfo{ChartName: "nginx", Description: "web", DefaultNamespace: "ingress-nginx"},
	)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	if result.Namespace != "staging" {
		t.Fatalf("expected staging namespace, got %q", result.Namespace)
	}
}

func TestGenerate_RequiresToolCall(t *testing.T) {
	mock := &mockLLMClient{
		chatCompletionFunc: func(ctx context.Context, messages []llm.Message, functions []llm.FunctionDefinition) (*llm.Message, error) {
			return &llm.Message{Content: "## namespace: default\nreplicas: 2"}, nil
		},
	}
	_, err := callValuesLLM(context.Background(), mock, valuesGenSystemPrompt, "prompt",
		&catalog.ChartInfo{ChartName: "x"},
	)
	if err == nil {
		t.Fatal("expected error when LLM returns text without tool call")
	}
}
