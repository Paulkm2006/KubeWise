package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/kubewise/kubewise/internal/platform/agentruntime"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/event"
	"github.com/kubewise/kubewise/internal/transport/http/ssestream"
)

var (
	ErrInteractionNotFound = errors.New("interaction not found")
	ErrInteractionClosed   = errors.New("agent no longer waiting for interaction")
)

type pendingInteraction struct {
	queryID string
	respCh  chan<- json.RawMessage
}

type SyncRequest struct {
	Query     string `json:"query"`
	QueryID   string `json:"query_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Cluster   string `json:"cluster,omitempty"`
}

type SyncResponse struct {
	QueryID string `json:"query_id"`
	Result  string `json:"result"`
}

type StreamRequest struct {
	Query     string
	QueryID   string
	SessionID string
	Cluster   string
}

type InteractionAnswer struct {
	InteractionID string
	Payload       json.RawMessage
}

type ChatService struct {
	agent   agentruntime.ChatPort
	mu      sync.RWMutex
	pending map[string]*pendingInteraction
}

func NewChatService(agent agentruntime.ChatPort) *ChatService {
	return &ChatService{
		agent:   agent,
		pending: make(map[string]*pendingInteraction),
	}
}

func (s *ChatService) QuerySync(_ context.Context, req SyncRequest) (*SyncResponse, error) {
	if req.Query == "" {
		return nil, errors.New("query is required")
	}
	result, err := s.agent.HandleQuery(buildQuery(req.Query, req.Cluster))
	if err != nil {
		return nil, err
	}
	queryID := req.QueryID
	if queryID == "" {
		queryID = fmt.Sprintf("q-%s", uuid.New().String()[:8])
	}
	return &SyncResponse{QueryID: queryID, Result: result}, nil
}

func (s *ChatService) Stream(ctx context.Context, req StreamRequest, sse *ssestream.SSEWriter) error {
	if req.Query == "" {
		return errors.New("query is required")
	}

	queryID := req.QueryID
	if queryID == "" {
		queryID = fmt.Sprintf("q-%s", uuid.New().String()[:8])
	}

	eventCh := make(chan event.Event, 64)
	go func() {
		defer close(eventCh)
		_ = s.agent.HandleQueryStream(ctx, buildQuery(req.Query, req.Cluster), queryID, eventCh)
	}()

	defer s.cleanupPending(queryID)

	for ev := range eventCh {
		if ctx.Err() != nil {
			break
		}
		if err := s.bridgeEvent(sse, ev); err != nil {
			break
		}
	}
	return nil
}

func (s *ChatService) AnswerInteraction(_ context.Context, ans InteractionAnswer) error {
	if ans.InteractionID == "" {
		return errors.New("interaction_id is required")
	}
	payload := ans.Payload
	if len(payload) == 0 {
		payload = json.RawMessage("{}")
	}

	s.mu.Lock()
	pi, ok := s.pending[ans.InteractionID]
	if ok {
		delete(s.pending, ans.InteractionID)
	}
	s.mu.Unlock()

	if !ok {
		return ErrInteractionNotFound
	}

	select {
	case pi.respCh <- payload:
		return nil
	default:
		return ErrInteractionClosed
	}
}

func (s *ChatService) cleanupPending(queryID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, pi := range s.pending {
		if pi.queryID == queryID {
			delete(s.pending, id)
		}
	}
}

func buildQuery(query, cluster string) string {
	if cluster == "" {
		return query
	}
	return fmt.Sprintf("[cluster: %s] %s", cluster, query)
}
