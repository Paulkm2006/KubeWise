package diagnosis

import "testing"

func TestRingBufferBasic(t *testing.T) {
	rb := NewRingBuffer(3)
	rb.Push(StreamEvent{Type: "a"})
	rb.Push(StreamEvent{Type: "b"})
	rb.Push(StreamEvent{Type: "c"})

	events := rb.Drain()
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	rb.Push(StreamEvent{Type: "d"})
	events = rb.Drain()
	if len(events) != 3 || events[0].Type != "b" {
		t.Fatalf("expected [b,c,d], got %v", events)
	}
}
