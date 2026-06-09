package diagnosis

type Runner struct {
	active    map[string]*RingBuffer
	diagnoses map[string]*Diagnosis
}

func NewRunner() *Runner {
	return &Runner{
		active:    make(map[string]*RingBuffer),
		diagnoses: make(map[string]*Diagnosis),
	}
}

func (r *Runner) Start(id string) {
	r.active[id] = NewRingBuffer(100)
}

func (r *Runner) PushEvent(id string, ev StreamEvent) {
	if buf, ok := r.active[id]; ok {
		buf.Push(ev)
	}
}

func (r *Runner) GetBuffer(id string) *RingBuffer {
	return r.active[id]
}

func (r *Runner) Finish(id string) []StreamEvent {
	buf := r.active[id]
	delete(r.active, id)
	if buf == nil {
		return nil
	}
	return buf.Drain()
}
