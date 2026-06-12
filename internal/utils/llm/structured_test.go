package llm

import (
	"context"
	"testing"
)

type fakeClientPort struct {
	responses []string
	calls     int
}

func (f *fakeClientPort) Complete(_ context.Context, req CompletionRequest) (*CompletionResponse, error) {
	f.calls++
	idx := f.calls - 1
	if idx >= len(f.responses) {
		idx = len(f.responses) - 1
	}
	return &CompletionResponse{
		Message: Message{Role: "assistant", Content: f.responses[idx]},
	}, nil
}

func TestCompleteJSONExtractsFencedJSON(t *testing.T) {
	client := &fakeClientPort{responses: []string{"```json\n{\"name\":\"nginx\"}\n```"}}
	var out struct {
		Name string `json:"name"`
	}

	resp, err := CompleteJSON(context.Background(), client, CompletionRequest{
		Messages: []Message{{Role: "user", Content: "return name"}},
	}, nil, &out)
	if err != nil {
		t.Fatalf("CompleteJSON() err = %v", err)
	}
	if resp.ParseError != "" {
		t.Fatalf("unexpected parse error: %s", resp.ParseError)
	}
	if out.Name != "nginx" {
		t.Fatalf("expected nginx, got %q", out.Name)
	}
}

func TestCompleteJSONRetriesOnceAfterMalformedJSON(t *testing.T) {
	client := &fakeClientPort{responses: []string{"not json", "{\"ok\":true}"}}
	var out struct {
		OK bool `json:"ok"`
	}

	_, err := CompleteJSON(context.Background(), client, CompletionRequest{
		Messages: []Message{{Role: "user", Content: "return ok"}},
	}, nil, &out)
	if err != nil {
		t.Fatalf("CompleteJSON() err = %v", err)
	}
	if client.calls != 2 {
		t.Fatalf("expected 2 calls, got %d", client.calls)
	}
	if !out.OK {
		t.Fatal("expected repaired JSON to set ok=true")
	}
}
