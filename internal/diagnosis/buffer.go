package diagnosis

import "sync"

type RingBuffer struct {
	mu     sync.RWMutex
	data   []StreamEvent
	size   int
	cursor int
	count  int
}

func NewRingBuffer(capacity int) *RingBuffer {
	return &RingBuffer{
		data: make([]StreamEvent, capacity),
		size: capacity,
	}
}

func (rb *RingBuffer) Push(e StreamEvent) bool {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	evicted := rb.count >= rb.size

	rb.data[rb.cursor] = e
	rb.cursor = (rb.cursor + 1) % rb.size
	if rb.count < rb.size {
		rb.count++
	}

	return !evicted
}

func (rb *RingBuffer) Drain() []StreamEvent {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	if rb.count == 0 {
		return nil
	}
	out := make([]StreamEvent, 0, rb.count)
	start := rb.cursor - rb.count
	if start < 0 {
		start += rb.size
	}
	for i := 0; i < rb.count; i++ {
		idx := (start + i) % rb.size
		out = append(out, rb.data[idx])
	}
	return out
}
