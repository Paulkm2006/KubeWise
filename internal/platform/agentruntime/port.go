// Package agentruntime is the shared kernel for LLM agent execution.
// Bounded contexts depend on context-specific ports, not router/sub-agent internals.
package agentruntime

import (
	"context"

	"github.com/kubewise/kubewise/internal/platform/agentruntime/event"
)

// ChatPort is the anti-corruption boundary for the Conversation context.
type ChatPort interface {
	HandleQuery(query string) (string, error)
	HandleQueryStream(ctx context.Context, query, queryID string, eventCh chan<- event.Event) error
}

// DiagnosisRunner is the anti-corruption boundary for the Diagnosis context.
type DiagnosisRunner interface {
	DiagnosePod(ctx context.Context, params DiagnoseParams, queryID string, events chan<- ProgressEvent) error
}

// DiagnoseParams identifies a pod to diagnose on a cluster.
type DiagnoseParams struct {
	Cluster   string
	Namespace string
	Pod       string
}

// ProgressEvent is a bounded-context-safe view of agent execution progress.
type ProgressEvent struct {
	Type        string
	Message     string
	Summary     string
	Detail      string
	Result      string
	TokenIn     int
	TokenOut    int
	ElapsedMs   int
	PayloadKind string
	PayloadJSON string
}
