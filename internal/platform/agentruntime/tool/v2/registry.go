package v2

import (
	"fmt"
	"sort"
	"sync"

	"github.com/kubewise/kubewise/internal/utils/llm"
)

type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

func (r *Registry) Register(t Tool) error {
	if t == nil {
		return fmt.Errorf("register nil tool")
	}
	meta := t.Meta()
	if meta.Name == "" {
		return fmt.Errorf("tool name is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tools[meta.Name]; exists {
		return fmt.Errorf("tool %s already registered", meta.Name)
	}
	r.tools[meta.Name] = t
	return nil
}

func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (r *Registry) Definitions(names []string) []llm.FunctionDefinition {
	allowed := make(map[string]struct{}, len(names))
	for _, name := range names {
		allowed[name] = struct{}{}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	defs := make([]llm.FunctionDefinition, 0, len(allowed))
	for name, t := range r.tools {
		if len(allowed) > 0 {
			if _, ok := allowed[name]; !ok {
				continue
			}
		}
		defs = append(defs, t.Meta().ToFunctionDefinition())
	}
	sort.Slice(defs, func(i, j int) bool { return defs[i].Name < defs[j].Name })
	return defs
}
