package operation

import (
	"context"
	"testing"
)

func TestStepToToolCall(t *testing.T) {
	t.Run("scale", func(t *testing.T) {
		replicas := int32(5)
		step := OperationStep{
			StepIndex: 1, OperationType: "scale",
			ResourceKind: "Deployment", ResourceName: "nginx", Namespace: "default",
			Replicas: &replicas, Description: "扩容 nginx",
		}
		toolName, args, err := stepToToolCall(step)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if toolName != "scale_resource" {
			t.Errorf("expected scale_resource, got %s", toolName)
		}
		if args["namespace"] != "default" {
			t.Errorf("wrong namespace: %v", args["namespace"])
		}
		if args["replicas"] != float64(5) {
			t.Errorf("wrong replicas: %v", args["replicas"])
		}
	})

	t.Run("restart", func(t *testing.T) {
		step := OperationStep{StepIndex: 1, OperationType: "restart",
			ResourceKind: "Deployment", ResourceName: "api", Namespace: "prod"}
		toolName, _, err := stepToToolCall(step)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if toolName != "restart_resource" {
			t.Errorf("expected restart_resource, got %s", toolName)
		}
	})

	t.Run("delete", func(t *testing.T) {
		step := OperationStep{StepIndex: 1, OperationType: "delete",
			ResourceKind: "Pod", ResourceName: "bad-pod", Namespace: "default",
			Group: "", Version: "v1", Resource: "pods"}
		toolName, args, err := stepToToolCall(step)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if toolName != "delete_resource" {
			t.Errorf("expected delete_resource, got %s", toolName)
		}
		if args["resource"] != "pods" {
			t.Errorf("wrong resource: %v", args["resource"])
		}
	})

	t.Run("cordon_drain", func(t *testing.T) {
		step := OperationStep{StepIndex: 1, OperationType: "cordon_drain",
			ResourceKind: "Node", ResourceName: "node-1", Action: "drain"}
		toolName, args, err := stepToToolCall(step)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if toolName != "cordon_drain_node" {
			t.Errorf("expected cordon_drain_node, got %s", toolName)
		}
		if args["action"] != "drain" {
			t.Errorf("wrong action: %v", args["action"])
		}
	})

	t.Run("unknown operation returns error", func(t *testing.T) {
		step := OperationStep{OperationType: "unknown"}
		_, _, err := stepToToolCall(step)
		if err == nil {
			t.Error("expected error for unknown operation type")
		}
	})
}

func TestChannelConfirmationHandlerConfirm(t *testing.T) {
	handler := NewChannelConfirmationHandler()
	ctx := context.Background()
	step := OperationStep{StepIndex: 1, OperationType: "scale",
		ResourceKind: "Deployment", ResourceName: "nginx", Namespace: "default"}

	go func() {
		req := <-handler.Requests
		if req.Step.ResourceName != "nginx" {
			t.Errorf("expected nginx, got %s", req.Step.ResourceName)
		}
		handler.Responses <- ConfirmResponse{Confirmed: true}
	}()

	confirmed, correction, err := handler.Confirm(ctx, step, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !confirmed {
		t.Error("expected confirmed=true")
	}
	if correction != "" {
		t.Errorf("expected empty correction, got %q", correction)
	}
}

func TestChannelConfirmationHandlerCorrection(t *testing.T) {
	handler := NewChannelConfirmationHandler()
	ctx := context.Background()
	step := OperationStep{StepIndex: 1, OperationType: "scale", ResourceName: "nginx"}

	go func() {
		<-handler.Requests
		handler.Responses <- ConfirmResponse{Confirmed: false, Correction: "改为 10 个副本"}
	}()

	confirmed, correction, err := handler.Confirm(ctx, step, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if confirmed {
		t.Error("expected confirmed=false")
	}
	if correction != "改为 10 个副本" {
		t.Errorf("expected correction text, got %q", correction)
	}
}

func TestExecuteConfirmPath(t *testing.T) {
	handler := NewChannelConfirmationHandler()
	called := false
	writeReg := &mockRegistry{
		executeFn: func(name string, args map[string]any) (string, error) {
			called = true
			if name != "scale_resource" {
				t.Errorf("expected scale_resource, got %s", name)
			}
			return "ok", nil
		},
	}

	replicas := int32(5)
	steps := []OperationStep{{
		StepIndex: 1, OperationType: "scale",
		ResourceKind: "Deployment", ResourceName: "nginx", Namespace: "default",
		Replicas: &replicas, Description: "扩容",
	}}

	ctx := context.Background()
	agent := &Agent{confirmHandler: handler, writeRegistry: writeReg}

	go func() {
		<-handler.Requests
		handler.Responses <- ConfirmResponse{Confirmed: true}
	}()

	summary, err := agent.execute(ctx, steps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected write tool to be called")
	}
	if summary == "" {
		t.Error("expected non-empty summary")
	}
}

func TestExecuteSkipPath(t *testing.T) {
	handler := NewChannelConfirmationHandler()
	called := false
	writeReg := &mockRegistry{
		executeFn: func(name string, args map[string]any) (string, error) {
			called = true
			return "ok", nil
		},
	}

	replicas := int32(3)
	steps := []OperationStep{{
		StepIndex: 1, OperationType: "scale", ResourceKind: "Deployment",
		ResourceName: "nginx", Namespace: "default", Replicas: &replicas,
	}}

	ctx := context.Background()
	agent := &Agent{confirmHandler: handler, writeRegistry: writeReg}

	go func() {
		<-handler.Requests
		handler.Responses <- ConfirmResponse{Confirmed: false, Correction: ""}
	}()

	_, err := agent.execute(ctx, steps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Error("expected write tool NOT to be called for skipped step")
	}
}

// mockRegistry satisfies the writeRegistryI interface.
type mockRegistry struct {
	executeFn func(name string, args map[string]any) (string, error)
}

func (m *mockRegistry) GetTool(name string) (toolExecutor, bool) {
	return &mockTool{name: name, fn: m.executeFn}, true
}

type mockTool struct {
	name string
	fn   func(name string, args map[string]any) (string, error)
}

func (m *mockTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	return m.fn(m.name, args)
}
