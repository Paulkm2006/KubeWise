package api

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/kubewise/kubewise/pkg/agent/router"
	"github.com/kubewise/kubewise/pkg/agent/stream"
	"github.com/kubewise/kubewise/pkg/agent/supervisor"
	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/llm"
	"github.com/kubewise/kubewise/pkg/tui/session"
)

// StreamQuerier abstracts the router agent for testability.
type StreamQuerier interface {
	HandleQuery(query string) (string, error)
	HandleQueryStream(ctx context.Context, query, queryID string, eventCh chan<- stream.Event) error
}

type pendingConfirm struct {
	queryID string
	respCh  chan<- any
}

type pendingInteraction struct {
	queryID string
	respCh  chan<- json.RawMessage
}

type Handler struct {
	querier             StreamQuerier
	sessionStore        *session.Store
	mu                  sync.RWMutex
	pendingConfirms     map[string]*pendingConfirm
	pendingInteractions map[string]*pendingInteraction
}

// NewHandler creates a Handler with real K8s/LLM clients.
func NewHandler(k8sClient *k8s.Client, llmClient *llm.Client, maxSteps int, supervisorCfg supervisor.Config) (*Handler, error) {
	routerAgent, err := router.New(k8sClient, llmClient, maxSteps, supervisorCfg)
	if err != nil {
		return nil, err
	}
	store, err := session.NewStore()
	if err != nil {
		return nil, err
	}
	return &Handler{
		querier:             routerAgent,
		sessionStore:        store,
		pendingConfirms:     make(map[string]*pendingConfirm),
		pendingInteractions: make(map[string]*pendingInteraction),
	}, nil
}

// NewHandlerWithDeps creates a Handler with custom dependencies (for testing).
func NewHandlerWithDeps(querier StreamQuerier, store *session.Store) *Handler {
	return &Handler{
		querier:             querier,
		sessionStore:        store,
		pendingConfirms:     make(map[string]*pendingConfirm),
		pendingInteractions: make(map[string]*pendingInteraction),
	}
}

func (h *Handler) cleanupPendingConfirms(queryID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, pc := range h.pendingConfirms {
		if pc.queryID == queryID {
			delete(h.pendingConfirms, id)
		}
	}
	for id, pi := range h.pendingInteractions {
		if pi.queryID == queryID {
			delete(h.pendingInteractions, id)
		}
	}
}
