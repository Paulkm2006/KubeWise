package tool

import (
	"fmt"
	"sync"

	"github.com/kubewise/kubewise/pkg/llm"
)

// Registry 工具注册中心
type Registry struct {
	mu         sync.RWMutex
	tools      map[string]Tool
	metadata   map[string]ToolMetadata
	dependency ToolDependency
}

// NewRegistry 创建工具注册中心
func NewRegistry(dep ToolDependency) *Registry {
	return &Registry{
		tools:      make(map[string]Tool),
		metadata:   make(map[string]ToolMetadata),
		dependency: dep,
	}
}

// Register 注册工具元数据
func (r *Registry) Register(meta ToolMetadata) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.metadata[meta.Name]; exists {
		return fmt.Errorf("tool %s already registered", meta.Name)
	}

	r.metadata[meta.Name] = meta
	return nil
}

// LoadAll 实例化所有已注册的工具
func (r *Registry) LoadAll() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for name, meta := range r.metadata {
		if _, exists := r.tools[name]; exists {
			continue // 已经实例化
		}

		tool, err := meta.Factory(r.dependency)
		if err != nil {
			return fmt.Errorf("failed to create tool %s: %w", name, err)
		}

		r.tools[name] = tool
	}
	return nil
}

// GetTool 根据名称获取工具实例
func (r *Registry) GetTool(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tool, exists := r.tools[name]
	return tool, exists
}

// GetAllFunctionDefinitions 获取所有工具的Function Definition列表
func (r *Registry) GetAllFunctionDefinitions() []llm.FunctionDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var defs []llm.FunctionDefinition
	for _, meta := range r.metadata {
		defs = append(defs, meta.ToFunctionDefinition())
	}
	return defs
}

// GetToolMetadata 获取指定工具的元数据
func (r *Registry) GetToolMetadata(name string) (ToolMetadata, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	meta, exists := r.metadata[name]
	return meta, exists
}

// 全局注册函数，各工具包在init()中调用
var globalRegistryEntries []ToolMetadata

// RegisterGlobal 全局注册工具元数据，在init()中调用
func RegisterGlobal(meta ToolMetadata) {
	globalRegistryEntries = append(globalRegistryEntries, meta)
}

// LoadGlobalRegistry 加载全局注册的所有工具，创建Registry实例
func LoadGlobalRegistry(dep ToolDependency) (*Registry, error) {
	reg := NewRegistry(dep)
	for _, meta := range globalRegistryEntries {
		if err := reg.Register(meta); err != nil {
			return nil, err
		}
	}
	if err := reg.LoadAll(); err != nil {
		return nil, err
	}
	return reg, nil
}
