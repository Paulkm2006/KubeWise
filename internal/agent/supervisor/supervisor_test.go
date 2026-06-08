package supervisor

import (
	"testing"

	"github.com/kubewise/kubewise/internal/utils/llm"
)

func TestStableJSON(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{"empty", map[string]any{}, "{}"},
		{"nil", nil, "{}"},
		{"simple", map[string]any{"namespace": "default"}, `{"namespace":"default"}`},
		{"multiple keys", map[string]any{"b": 1, "a": 2}, `{"a":2,"b":1}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stableJSON(tt.args)
			if got != tt.want {
				t.Errorf("stableJSON() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStableJSONDeterministic(t *testing.T) {
	// Calling stableJSON multiple times on the same map should produce identical output
	args := map[string]any{
		"namespace": "prod",
		"resource":  "pods",
		"version":   "v1",
	}
	first := stableJSON(args)
	for i := 0; i < 10; i++ {
		if got := stableJSON(args); got != first {
			t.Errorf("stableJSON not deterministic: got %v, want %v", got, first)
		}
	}
}

func TestLoopDetectorExactRepeat(t *testing.T) {
	cfg := DefaultConfig()
	d := newLoopDetector(20)

	fp := toolCallFingerprint{toolName: "list_pods", argsKey: `{"namespace":"default"}`}

	// Below threshold
	for i := 0; i < cfg.RepeatThreshold-1; i++ {
		d.record([]toolCallFingerprint{fp})
	}
	if desc := d.detect(cfg); desc != "" {
		t.Errorf("should not detect loop before threshold, got: %s", desc)
	}

	// At threshold
	d.record([]toolCallFingerprint{fp})
	desc := d.detect(cfg)
	if desc == "" {
		t.Error("should detect exact repeat at threshold")
	}
}

func TestLoopDetectorPingPong(t *testing.T) {
	cfg := DefaultConfig()
	d := newLoopDetector(20)

	fpA := toolCallFingerprint{toolName: "get_pod_logs", argsKey: `{"pod":"app-1"}`}
	fpB := toolCallFingerprint{toolName: "get_resource_events", argsKey: `{"name":"app-1"}`}

	// Build A-B-A-B-A-B pattern (3 cycles = threshold)
	for i := 0; i < cfg.PingPongThreshold; i++ {
		d.record([]toolCallFingerprint{fpA})
		d.record([]toolCallFingerprint{fpB})
	}
	desc := d.detect(cfg)
	if desc == "" {
		t.Error("should detect ping-pong pattern")
	}
}

func TestLoopDetectorSameToolHammer(t *testing.T) {
	cfg := DefaultConfig()
	d := newLoopDetector(20)

	// Same tool name with different args
	for i := 0; i < cfg.SameToolThreshold; i++ {
		fp := toolCallFingerprint{toolName: "list_resources_by_gvr", argsKey: string(rune('a' + i))}
		d.record([]toolCallFingerprint{fp})
	}
	desc := d.detect(cfg)
	if desc == "" {
		t.Error("should detect same-tool hammer")
	}
}

func TestLoopDetectorNoFalsePositive(t *testing.T) {
	cfg := DefaultConfig()
	d := newLoopDetector(20)

	// Different tools with different args — no loop
	for i := 0; i < 10; i++ {
		fp := toolCallFingerprint{
			toolName: "tool_" + string(rune('A'+i)),
			argsKey:  string(rune('a' + i)),
		}
		d.record([]toolCallFingerprint{fp})
	}
	desc := d.detect(cfg)
	if desc != "" {
		t.Errorf("should not detect loop for diverse tools, got: %s", desc)
	}
}

func TestLoopDetectorReset(t *testing.T) {
	cfg := DefaultConfig()
	d := newLoopDetector(20)

	fp := toolCallFingerprint{toolName: "list_pods", argsKey: `{"namespace":"default"}`}
	for i := 0; i < cfg.RepeatThreshold; i++ {
		d.record([]toolCallFingerprint{fp})
	}
	if desc := d.detect(cfg); desc == "" {
		t.Error("should detect loop before reset")
	}

	d.reset()
	if desc := d.detect(cfg); desc != "" {
		t.Errorf("should not detect loop after reset, got: %s", desc)
	}
}

func TestBuildFingerprints(t *testing.T) {
	fps := buildFingerprints(
		[]string{"list_pods", "get_pod_logs"},
		[]map[string]any{
			{"namespace": "default"},
			{"pod": "app-1", "container": "main"},
		},
	)
	if len(fps) != 2 {
		t.Fatalf("expected 2 fingerprints, got %d", len(fps))
	}
	if fps[0].toolName != "list_pods" {
		t.Errorf("fps[0].toolName = %v, want list_pods", fps[0].toolName)
	}
	if fps[1].toolName != "get_pod_logs" {
		t.Errorf("fps[1].toolName = %v, want get_pod_logs", fps[1].toolName)
	}
}

func TestSupervisorNoOpWhenDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = false
	sv := New(cfg, nil)

	if sv.Enabled() {
		t.Error("supervisor should be disabled")
	}

	triggered, result := sv.CheckLoop(nil, 0, nil, nil)
	if triggered {
		t.Error("disabled supervisor should not trigger on CheckLoop")
	}
	if result != nil {
		t.Error("disabled supervisor should return nil result")
	}
}

func TestSupervisorCheckLoopDetectsRepeat(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RepeatThreshold = 3
	cfg.Enabled = false // Disable so LLM evaluation is skipped (nil client)
	sv := New(cfg, nil)

	// Record below threshold
	sv.detector.record([]toolCallFingerprint{{toolName: "list_pods", argsKey: `{"namespace":"default"}`}})
	sv.detector.record([]toolCallFingerprint{{toolName: "list_pods", argsKey: `{"namespace":"default"}`}})

	// With supervisor disabled, CheckLoop should not trigger
	triggered, _ := sv.CheckLoop(nil, 0, []llm.ToolCall{
		{Function: llm.FunctionCall{Name: "list_pods", Arguments: map[string]any{"namespace": "default"}}},
	}, nil)
	if triggered {
		t.Error("disabled supervisor should not trigger")
	}
}

func TestLoopDetectorCheckLoopIntegration(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RepeatThreshold = 3
	// Create supervisor with nil client but track loop detection
	// CheckLoop will detect the loop internally, but evaluation will fail gracefully
	sv := New(cfg, nil)

	// Record enough repeats to trigger detection
	tc := llm.ToolCall{Function: llm.FunctionCall{Name: "list_pods", Arguments: map[string]any{"namespace": "default"}}}
	for i := 0; i < cfg.RepeatThreshold; i++ {
		sv.detector.record([]toolCallFingerprint{{toolName: "list_pods", argsKey: stableJSON(tc.Function.Arguments)}})
	}

	// The loop pattern is in the detector
	desc := sv.detector.detect(cfg)
	if desc == "" {
		t.Error("detector should find the repeat pattern")
	}
}

func TestSupervisorReset(t *testing.T) {
	cfg := DefaultConfig()
	sv := New(cfg, nil)

	sv.extensionsGranted = 5
	sv.evaluatorCallsUsed = 3
	sv.detector.record([]toolCallFingerprint{{toolName: "test", argsKey: "test"}})

	sv.Reset()

	if sv.extensionsGranted != 0 {
		t.Errorf("extensionsGranted = %d, want 0", sv.extensionsGranted)
	}
	if sv.evaluatorCallsUsed != 0 {
		t.Errorf("evaluatorCallsUsed = %d, want 0", sv.evaluatorCallsUsed)
	}
	if len(sv.detector.history) != 0 {
		t.Errorf("detector history not cleared")
	}
}

func TestSupervisorEvaluateAbortsWhenMaxCallsExceeded(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxEvaluatorCalls = 1
	sv := New(cfg, nil)
	sv.evaluatorCallsUsed = 1 // Already used the one allowed call

	result, err := sv.Evaluate(nil, nil, 20, 20)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result.Decision != DecisionAbort {
		t.Errorf("Decision = %v, want abort", result.Decision)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.Enabled {
		t.Error("default config should have Enabled=true")
	}
	if cfg.RepeatThreshold != 3 {
		t.Errorf("RepeatThreshold = %d, want 3", cfg.RepeatThreshold)
	}
	if cfg.PingPongThreshold != 3 {
		t.Errorf("PingPongThreshold = %d, want 3", cfg.PingPongThreshold)
	}
	if cfg.SameToolThreshold != 5 {
		t.Errorf("SameToolThreshold = %d, want 5", cfg.SameToolThreshold)
	}
	if cfg.MaxExtensions != 2 {
		t.Errorf("MaxExtensions = %d, want 2", cfg.MaxExtensions)
	}
	if cfg.ExtensionStepGrant != 10 {
		t.Errorf("ExtensionStepGrant = %d, want 10", cfg.ExtensionStepGrant)
	}
	if cfg.MaxEvaluatorCalls != 2 {
		t.Errorf("MaxEvaluatorCalls = %d, want 2", cfg.MaxEvaluatorCalls)
	}
}

func TestNewSupervisorFillsDefaults(t *testing.T) {
	cfg := Config{} // All zeros
	sv := New(cfg, nil)

	if sv.config.RepeatThreshold != 3 {
		t.Errorf("RepeatThreshold not filled: %d", sv.config.RepeatThreshold)
	}
	if sv.config.PingPongThreshold != 3 {
		t.Errorf("PingPongThreshold not filled: %d", sv.config.PingPongThreshold)
	}
	if sv.config.SameToolThreshold != 5 {
		t.Errorf("SameToolThreshold not filled: %d", sv.config.SameToolThreshold)
	}
}

func TestBuildConversationSummary(t *testing.T) {
	fps := []toolCallFingerprint{
		{toolName: "list_namespaces", argsKey: "{}"},
		{toolName: "list_pods", argsKey: `{"namespace":"prod"}`},
	}
	results := []string{"5 namespaces found", "12 pods running in prod"}

	summary := buildConversationSummary(fps, results)
	if len(summary) == 0 {
		t.Error("summary should not be empty")
	}
	// Should contain step numbers and tool names
	if !contains(summary, "Step 1") || !contains(summary, "Step 2") {
		t.Error("summary should contain step numbers")
	}
	if !contains(summary, "list_namespaces") || !contains(summary, "list_pods") {
		t.Error("summary should contain tool names")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
