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

	"github.com/kubewise/kubewise/pkg/agent/stream"
	"github.com/kubewise/kubewise/pkg/tui/events"
	"github.com/kubewise/kubewise/pkg/tui/session"
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

func newTestStore(t *testing.T) *session.Store {
	t.Helper()
	return &session.Store{Dir: t.TempDir()}
}

func setupEcho(h *Handler) *echo.Echo {
	e := echo.New()
	e.GET("/health", h.Health)
	v1 := e.Group("/api/v1")
	v1.POST("/chat", h.ChatSync)
	v1.GET("/chat/stream", h.ChatStream)
	v1.POST("/chat/confirm", h.ChatConfirm)
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

func TestChatStreamSSE(t *testing.T) {
	q := &mockStreamQuerier{
		handleQueryStream: func(ctx context.Context, query, queryID string, eventCh chan<- stream.Event) error {
			eventCh <- stream.Legacy{TUI: events.PhaseEvent{QueryID: queryID, Phase: "thinking"}}
			eventCh <- stream.Legacy{TUI: events.RenderTextEvent{QueryID: queryID, Text: "hello"}}
			eventCh <- stream.Legacy{TUI: events.StreamDoneEvent{QueryID: queryID, Result: "hello"}}
			return nil
		},
	}
	h := NewHandlerWithDeps(q, newTestStore(t))
	e := setupEcho(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/chat/stream?query=test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("expected text/event-stream, got %s", ct)
	}
	body := rec.Body.String()
	for _, ev := range []string{"event: phase", "event: render_text", "event: stream_done"} {
		if !strings.Contains(body, ev) {
			t.Errorf("missing %s", ev)
		}
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

func TestConfirmFlow(t *testing.T) {
	respCh := make(chan any, 1)
	q := &mockStreamQuerier{
		handleQueryStream: func(ctx context.Context, query, queryID string, eventCh chan<- stream.Event) error {
			eventCh <- stream.Legacy{TUI: events.ConfirmRequestEvent{
				QueryID: queryID, Step: map[string]string{"op": "scale"},
				TotalSteps: 1, RespCh: respCh,
			}}
			select {
			case <-respCh:
			case <-ctx.Done():
			}
			eventCh <- stream.Legacy{TUI: events.StreamDoneEvent{QueryID: queryID, Result: "done"}}
			return nil
		},
	}
	h := NewHandlerWithDeps(q, newTestStore(t))
	e := setupEcho(h)

	// Start stream in background
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/chat/stream?query=scale", nil)
	done := make(chan struct{})
	go func() {
		defer close(done)
		e.ServeHTTP(rec, req)
	}()

	// Wait for confirm_request
	var confirmID string
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for confirm_request")
		case <-done:
			t.Fatal("stream ended before confirm_request")
		default:
		}
		body := rec.Body.String()
		if strings.Contains(body, "event: confirm_request") {
			for _, line := range strings.Split(body, "\n") {
				if strings.HasPrefix(line, "data: ") {
					var data ConfirmRequestData
					if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &data) == nil && data.ConfirmID != "" {
						confirmID = data.ConfirmID
					}
				}
			}
			if confirmID != "" {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	// POST confirmation
	confirmBody := `{"confirm_id":"` + confirmID + `","confirmed":true}`
	cReq := httptest.NewRequest(http.MethodPost, "/api/v1/chat/confirm", strings.NewReader(confirmBody))
	cReq.Header.Set("Content-Type", "application/json")
	cRec := httptest.NewRecorder()
	e.ServeHTTP(cRec, cReq)

	if cRec.Code != http.StatusOK {
		t.Fatalf("confirm: expected 200, got %d: %s", cRec.Code, cRec.Body.String())
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
			eventCh <- stream.Legacy{TUI: events.StreamDoneEvent{QueryID: queryID, Result: "done"}}
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

func TestConfirmNotFound(t *testing.T) {
	h := NewHandlerWithDeps(&mockStreamQuerier{}, newTestStore(t))
	e := setupEcho(h)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/chat/confirm", strings.NewReader(`{"confirm_id":"bad","confirmed":true}`))
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

func TestSSEAllEventTypes(t *testing.T) {
	q := &mockStreamQuerier{
		handleQueryStream: func(ctx context.Context, query, queryID string, eventCh chan<- stream.Event) error {
			eventCh <- stream.Legacy{TUI: events.AgentStartEvent{QueryID: queryID, AgentName: "query"}}
			eventCh <- stream.Legacy{TUI: events.ToolCallEvent{QueryID: queryID, ToolName: "list_pods", Step: 1}}
			eventCh <- stream.Legacy{TUI: events.ToolDoneEvent{QueryID: queryID, ToolName: "list_pods", Step: 1, Elapsed: time.Second}}
			eventCh <- stream.Legacy{TUI: events.RenderTableEvent{QueryID: queryID, Headers: []string{"Name"}, Rows: [][]string{{"pod-1"}}}}
			eventCh <- stream.Legacy{TUI: events.RenderCodeEvent{QueryID: queryID, Language: "yaml", Content: "apiVersion: v1"}}
			eventCh <- stream.Legacy{TUI: events.RenderKVEvent{QueryID: queryID, Pairs: []events.KVPair{{Key: "k", Value: "v"}}}}
			eventCh <- stream.Legacy{TUI: events.RenderListEvent{QueryID: queryID, Items: []events.ListItem{{Status: "ok", Text: "t"}}}}
			eventCh <- stream.Legacy{TUI: events.SupervisorEvent{QueryID: queryID, Reason: "loop", Decision: "continue", Detail: "d"}}
			eventCh <- stream.Legacy{TUI: events.StreamDoneEvent{QueryID: queryID, Result: "done"}}
			return nil
		},
	}
	h := NewHandlerWithDeps(q, newTestStore(t))
	e := setupEcho(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/chat/stream?query=test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	body := rec.Body.String()
	for _, ev := range []string{
		"event: agent_start", "event: tool_call", "event: tool_done",
		"event: render_table", "event: render_code", "event: render_kv",
		"event: render_list", "event: supervisor", "event: stream_done",
	} {
		if !strings.Contains(body, ev) {
			t.Errorf("missing %s", ev)
		}
	}
}

func TestCleanupPendingConfirms(t *testing.T) {
	ch := make(chan any, 1)
	jch := make(chan json.RawMessage, 1)
	h := &Handler{
		pendingConfirms: map[string]*pendingConfirm{
			"a": {queryID: "q1", respCh: ch},
			"b": {queryID: "q2", respCh: ch},
			"c": {queryID: "q1", respCh: ch},
		},
		pendingInteractions: map[string]*pendingInteraction{
			"d": {queryID: "q1", respCh: jch},
			"e": {queryID: "q2", respCh: jch},
		},
	}
	h.cleanupPendingConfirms("q1")
	if len(h.pendingConfirms) != 1 {
		t.Fatalf("expected 1 pending confirm, got %d", len(h.pendingConfirms))
	}
	if _, ok := h.pendingConfirms["b"]; !ok {
		t.Fatal("expected 'b' to remain")
	}
	if len(h.pendingInteractions) != 1 {
		t.Fatalf("expected 1 pending interaction, got %d", len(h.pendingInteractions))
	}
	if _, ok := h.pendingInteractions["e"]; !ok {
		t.Fatal("expected interaction 'e' to remain")
	}
}
