package ssestream

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type SSEWriter struct {
	w     http.ResponseWriter
	flush http.Flusher
}

func NewSSEWriter(w http.ResponseWriter) (*SSEWriter, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("streaming unsupported: ResponseWriter does not implement http.Flusher")
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	return &SSEWriter{w: w, flush: flusher}, nil
}

func (s *SSEWriter) WriteEvent(event string, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal SSE data: %w", err)
	}
	if event != "" {
		fmt.Fprintf(s.w, "event: %s\n", event)
	}
	fmt.Fprintf(s.w, "data: %s\n\n", jsonData)
	s.flush.Flush()
	return nil
}

// WriteEventWithID writes an SSE event with an explicit event ID for
// Last-Event-ID / reconnect support. Browser EventSource will
// automatically send Last-Event-ID on reconnection.
func (s *SSEWriter) WriteEventWithID(event string, id int, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal SSE data: %w", err)
	}
	if event != "" {
		fmt.Fprintf(s.w, "event: %s\n", event)
	}
	fmt.Fprintf(s.w, "id: %d\n", id)
	fmt.Fprintf(s.w, "data: %s\n\n", jsonData)
	s.flush.Flush()
	return nil
}

func (s *SSEWriter) WriteComment(text string) {
	fmt.Fprintf(s.w, ": %s\n\n", text)
	s.flush.Flush()
}
