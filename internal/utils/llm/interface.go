package llm

import "context"

// ClientPort is the platform-level LLM boundary used by agent runtimes.
type ClientPort interface {
	Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
}

// JSONCompleter is implemented by clients that can produce validated JSON.
type JSONCompleter interface {
	CompleteJSON(ctx context.Context, req CompletionRequest, schema map[string]any, dest any) (*CompletionResponse, error)
}
