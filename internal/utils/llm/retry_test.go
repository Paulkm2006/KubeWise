package llm

import (
	"context"
	"errors"
	"testing"
	"time"
)

type flakyClient struct {
	errs  []error
	calls int
}

func (f *flakyClient) Complete(context.Context, CompletionRequest) (*CompletionResponse, error) {
	idx := f.calls
	f.calls++
	if idx >= len(f.errs) {
		idx = len(f.errs) - 1
	}
	if f.errs[idx] != nil {
		return nil, f.errs[idx]
	}
	return &CompletionResponse{Message: Message{Role: "assistant", Content: "ok"}}, nil
}

func TestIsTransientError(t *testing.T) {
	if !IsTransientError(errors.New("chat completion failed: 429 Too Many Requests")) {
		t.Fatal("expected 429 to be transient")
	}
	if IsTransientError(errors.New("chat completion failed: 401 Unauthorized")) {
		t.Fatal("expected 401 to be non-transient")
	}
}

func TestCompleteWithRetryRecoversFromTransientFailure(t *testing.T) {
	client := &flakyClient{errs: []error{
		errors.New("chat completion failed: 503 service unavailable"),
		nil,
	}}
	resp, err := CompleteWithRetry(context.Background(), client, CompletionRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	}, RetryPolicy{MaxAttempts: 3, Backoff: []time.Duration{1 * time.Millisecond}})
	if err != nil {
		t.Fatalf("CompleteWithRetry() err = %v", err)
	}
	if resp.Message.Content != "ok" {
		t.Fatalf("unexpected response: %#v", resp.Message)
	}
	if client.calls != 2 {
		t.Fatalf("expected 2 calls, got %d", client.calls)
	}
}

func TestCompleteWithRetryDoesNotRetryPermanentFailure(t *testing.T) {
	client := &flakyClient{errs: []error{errors.New("chat completion failed: 401 unauthorized")}}
	_, err := CompleteWithRetry(context.Background(), client, CompletionRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	}, RetryPolicy{MaxAttempts: 3, Backoff: []time.Duration{1 * time.Millisecond}})
	if err == nil {
		t.Fatal("expected permanent error")
	}
	if client.calls != 1 {
		t.Fatalf("expected single call, got %d", client.calls)
	}
}
