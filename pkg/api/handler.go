package api

import (
	"context"
	"encoding/json"
	"sync"

	appsession "github.com/kubewise/kubewise/pkg/session"
	"github.com/kubewise/kubewise/pkg/session/store"
	"github.com/kubewise/kubewise/pkg/stream"
)

// StreamQuerier abstracts the router agent for testability.
type StreamQuerier interface {
	HandleQuery(query string) (string, error)
	HandleQueryStream(ctx context.Context, query, queryID string, eventCh chan<- stream.Event) error
}

type pendingInteraction struct {
	queryID string
	respCh  chan<- json.RawMessage
}

type Handler struct {
	querier             StreamQuerier
	sessionStore        store.Store
	mu                  sync.RWMutex
	pendingInteractions map[string]*pendingInteraction
}

// NewHandler creates a Handler with real K8s/LLM clients.
func NewHandler(sess *appsession.Session) (*Handler, error) {
	routerAgent := sess.Router
	store, err := store.NewFileStore()
	if err != nil {
		return nil, err
	}
	return &Handler{
		querier:             routerAgent,
		sessionStore:        store,
		pendingInteractions: make(map[string]*pendingInteraction),
	}, nil
}

// NewHandlerWithDeps creates a Handler with custom dependencies (for testing).
func NewHandlerWithDeps(querier StreamQuerier, store store.Store) *Handler {
	return &Handler{
		querier:             querier,
		sessionStore:        store,
		pendingInteractions: make(map[string]*pendingInteraction),
	}
}

func (h *Handler) cleanupPendingInteractions(queryID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, pi := range h.pendingInteractions {
		if pi.queryID == queryID {
			delete(h.pendingInteractions, id)
		}
	}
}
