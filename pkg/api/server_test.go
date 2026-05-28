package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/kubewise/kubewise/pkg/session/store"
	"github.com/kubewise/kubewise/pkg/stream"
)

type mockStreamQuerier struct {
	handleQuery       func(string) (string, error)
	handleQueryStream func(ctx context.Context, query, queryID string, eventCh chan<- stream.Event) error
}

func (m *mockStreamQuerier) HandleQuery(q string) (string, error) {
	return m.handleQuery(q)
}

func (m *mockStreamQuerier) HandleQueryStream(ctx context.Context, query, queryID string, eventCh chan<- stream.Event) error {
	return m.handleQueryStream(ctx, query, queryID, eventCh)
}

func newTestStore(t *testing.T) store.Store {
	t.Helper()
	return &store.FileStore{Dir: t.TempDir()}
}

func setupEcho(h *Handler) *echo.Echo {
	e := echo.New()
	e.GET("/health", h.Health)
	v1 := e.Group("/api/v1")
	v1.POST("/chat", h.ChatSync)
	v1.GET("/chat/stream", h.ChatStream)
	v1.POST("/chat/interaction", h.ChatInteraction)
	v1.GET("/sessions", h.ListSessions)
	v1.POST("/sessions", h.CreateSession)
	v1.GET("/sessions/:id", h.GetSession)
	v1.DELETE("/sessions/:id", h.DeleteSession)
	return e
}

func TestHealth(t *testing.T) {
	h := NewHandlerWithDeps(&mockStreamQuerier{}, newTestStore(t))
	e := setupEcho(h)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp HealthResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Status != "ok" {
		t.Fatalf("expected status ok, got %s", resp.Status)
	}
}

func TestChatSync(t *testing.T) {
	q := &mockStreamQuerier{
		handleQuery: func(s string) (string, error) { return "result: " + s, nil },
	}
	h := NewHandlerWithDeps(q, newTestStore(t))
	e := setupEcho(h)

	body := `{"query":"list pods"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chat", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp ChatResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Result != "result: list pods" {
		t.Fatalf("unexpected result: %s", resp.Result)
	}
}

func TestChatSyncEmptyQuery(t *testing.T) {
	h := NewHandlerWithDeps(&mockStreamQuerier{}, newTestStore(t))
	e := setupEcho(h)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/chat", strings.NewReader(`{"query":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestChatStreamNoQuery(t *testing.T) {
	h := NewHandlerWithDeps(&mockStreamQuerier{}, newTestStore(t))
	e := setupEcho(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/chat/stream", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestChatStreamLLMTextDeltaSSE(t *testing.T) {
	q := &mockStreamQuerier{
		handleQueryStream: func(ctx context.Context, query, queryID string, eventCh chan<- stream.Event) error {
			eventCh <- stream.LLMTextDelta{QueryID: queryID, Delta: "hello"}
			eventCh <- stream.StreamDone{QueryID: queryID}
			return nil
		},
	}
	h := NewHandlerWithDeps(q, newTestStore(t))
	e := setupEcho(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/chat/stream?query=ping", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	lines := strings.Split(rec.Body.String(), "\n")
	for i := 0; i+1 < len(lines); i++ {
		if lines[i] != "event: llm_text_delta" {
			continue
		}
		if !strings.HasPrefix(lines[i+1], "data: ") {
			t.Fatalf("missing data line for llm_text_delta event: %q", lines[i+1])
		}

		raw := strings.TrimPrefix(lines[i+1], "data: ")
		var payload map[string]any
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			t.Fatalf("invalid llm_text_delta payload: %v", err)
		}

		queryID, ok := payload["query_id"].(string)
		if !ok || queryID == "" {
			t.Fatalf("expected non-empty query_id in payload: %s", raw)
		}
		delta, ok := payload["delta"].(string)
		if !ok || delta != "hello" {
			t.Fatalf("unexpected delta in payload: %s", raw)
		}
		if len(payload) != 2 {
			t.Fatalf("unexpected payload shape for llm_text_delta: %s", raw)
		}
		return
	}

	t.Fatalf("expected llm_text_delta event in stream, got body: %s", rec.Body.String())
}

func TestConfirmFlow(t *testing.T) {
	q := &mockStreamQuerier{
		handleQueryStream: func(ctx context.Context, query, queryID string, eventCh chan<- stream.Event) error {
			stepJSON := json.RawMessage(`{"operation_type":"scale"}`)
			respCh := make(chan json.RawMessage, 1)
			eventCh <- stream.InteractionRequest{
				QueryID: queryID, Kind: stream.KindOperationStep,
				Payload: stepJSON, TotalSteps: 1, RespCh: respCh,
			}
			select {
			case <-respCh:
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
				return context.DeadlineExceeded
			}
			eventCh <- stream.StreamDone{QueryID: queryID}
			return nil
		},
	}
	h := NewHandlerWithDeps(q, newTestStore(t))
	e := setupEcho(h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/chat/stream?query=scale", nil)
	done := make(chan struct{})
	go func() {
		defer close(done)
		e.ServeHTTP(rec, req)
	}()

	var interactionID string
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for interaction_request")
		case <-done:
			t.Fatal("stream ended before interaction_request")
		default:
		}
		body := rec.Body.String()
		if strings.Contains(body, "event: interaction_request") {
			for _, line := range strings.Split(body, "\n") {
				if strings.HasPrefix(line, "data: ") {
					var data InteractionRequestData
					if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &data) == nil && data.InteractionID != "" {
						interactionID = data.InteractionID
					}
				}
			}
			if interactionID != "" {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	ans := `{"interaction_id":"` + interactionID + `","payload":{"confirmed":true}}`
	cReq := httptest.NewRequest(http.MethodPost, "/api/v1/chat/interaction", strings.NewReader(ans))
	cReq.Header.Set("Content-Type", "application/json")
	cRec := httptest.NewRecorder()
	e.ServeHTTP(cRec, cReq)

	if cRec.Code != http.StatusOK {
		t.Fatalf("interaction: expected 200, got %d: %s", cRec.Code, cRec.Body.String())
	}
}

func TestInteractionOperationStepFlow(t *testing.T) {
	q := &mockStreamQuerier{
		handleQueryStream: func(ctx context.Context, query, queryID string, eventCh chan<- stream.Event) error {
			stepJSON := json.RawMessage(`{"step_index":1,"operation_type":"restart","resource_kind":"Deployment","resource_name":"demo","description":"bounce"}`)
			respCh := make(chan json.RawMessage, 1)
			eventCh <- stream.InteractionRequest{
				QueryID:    queryID,
				Kind:       stream.KindOperationStep,
				Payload:    stepJSON,
				TotalSteps: 3,
				RespCh:     respCh,
			}
			select {
			case <-respCh:
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
				return context.DeadlineExceeded
			}
			eventCh <- stream.StreamDone{QueryID: queryID}
			return nil
		},
	}
	h := NewHandlerWithDeps(q, newTestStore(t))
	e := setupEcho(h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/chat/stream?query=op", nil)
	done := make(chan struct{})
	go func() {
		defer close(done)
		e.ServeHTTP(rec, req)
	}()

	var interactionID string
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for interaction_request")
		case <-done:
			t.Fatal("stream ended before interaction_request")
		default:
		}
		body := rec.Body.String()
		if strings.Contains(body, "event: interaction_request") {
			for _, line := range strings.Split(body, "\n") {
				if strings.HasPrefix(line, "data: ") {
					var data InteractionRequestData
					if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &data) == nil && data.InteractionID != "" && data.Kind == string(stream.KindOperationStep) {
						interactionID = data.InteractionID
					}
				}
			}
			if interactionID != "" {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	ans := `{"interaction_id":"` + interactionID + `","payload":{"confirmed":true}}`
	cReq := httptest.NewRequest(http.MethodPost, "/api/v1/chat/interaction", strings.NewReader(ans))
	cReq.Header.Set("Content-Type", "application/json")
	cRec := httptest.NewRecorder()
	e.ServeHTTP(cRec, cReq)
	if cRec.Code != http.StatusOK {
		t.Fatalf("interaction: expected 200, got %d: %s", cRec.Code, cRec.Body.String())
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("stream did not complete")
	}
}

func TestInteractionNotFound(t *testing.T) {
	h := NewHandlerWithDeps(&mockStreamQuerier{}, newTestStore(t))
	e := setupEcho(h)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/chat/interaction", strings.NewReader(`{"interaction_id":"bad","payload":{}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestSessionCRUD(t *testing.T) {
	store := newTestStore(t)
	h := NewHandlerWithDeps(&mockStreamQuerier{}, store)
	e := setupEcho(h)

	// Create
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", strings.NewReader(`{"title":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created SessionResponse
	json.Unmarshal(rec.Body.Bytes(), &created)

	// List
	req = httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", rec.Code)
	}
	var list SessionListResponse
	json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(list.Sessions))
	}

	// Get
	req = httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+created.ID, nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", rec.Code)
	}

	// Delete
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/sessions/"+created.ID, nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify deleted
	req = httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+created.ID, nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get after delete: expected 404, got %d", rec.Code)
	}
}

func TestDeleteSessionNotFound(t *testing.T) {
	h := NewHandlerWithDeps(&mockStreamQuerier{}, newTestStore(t))
	e := setupEcho(h)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/sessions/nonexistent", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestCleanupPendingInteractions(t *testing.T) {
	jch := make(chan json.RawMessage, 1)
	h := &Handler{
		pendingInteractions: map[string]*pendingInteraction{
			"a": {queryID: "q1", respCh: jch},
			"b": {queryID: "q2", respCh: jch},
			"c": {queryID: "q1", respCh: jch},
		},
	}
	h.cleanupPendingInteractions("q1")
	if len(h.pendingInteractions) != 1 {
		t.Fatalf("expected 1 pending interaction, got %d", len(h.pendingInteractions))
	}
	if _, ok := h.pendingInteractions["b"]; !ok {
		t.Fatal("expected interaction 'b' to remain")
	}
}
