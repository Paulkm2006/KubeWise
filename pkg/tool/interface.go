package tool

import (
	"context"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/llm"
)

// Tool 所有工具必须实现的核心接口
type Tool interface {
	// Name 返回工具的唯一标识（下划线命名，如 list_persistent_volumes）
	Name() string

	// Description 返回工具的功能描述
	Description() string

	// Parameters 返回工具的参数定义（JSON Schema格式）
	Parameters() map[string]any

	// Execute 执行工具调用，参数是解析后的键值对
	Execute(ctx context.Context, args map[string]any) (string, error)
}

// ToolMetadata 工具元数据，用于注册和发现
type ToolMetadata struct {
	Name        string
	Description string
	Parameters  map[string]any
	Factory     func(dep any) (Tool, error) // 工具工厂函数，用于依赖注入
}

// ToolDependency 工具依赖注入的统一接口
type ToolDependency struct {
	K8sClient *k8s.Client
	// 可扩展其他依赖，如配置、其他客户端等
}

// ToFunctionDefinition 将ToolMetadata转换为llm.FunctionDefinition
func (m *ToolMetadata) ToFunctionDefinition() llm.FunctionDefinition {
	return llm.FunctionDefinition{
		Name:        m.Name,
		Description: m.Description,
		Parameters:  m.Parameters,
	}
}
