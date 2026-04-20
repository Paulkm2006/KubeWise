package query

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/llm"
	"github.com/kubewise/kubewise/pkg/tools"
	"github.com/kubewise/kubewise/pkg/types"
)

// Agent 查询Agent
type Agent struct {
	k8sClient *k8s.Client
	llmClient *llm.Client
	tools     *tools.QueryTools
}

// New 创建查询Agent
func New(k8sClient *k8s.Client, llmClient *llm.Client) *Agent {
	return &Agent{
		k8sClient: k8sClient,
		llmClient: llmClient,
		tools:     tools.NewQueryTools(k8sClient),
	}
}

// HandleQuery 处理查询请求
func (a *Agent) HandleQuery(ctx context.Context, userQuery string, entities types.Entities) (string, error) {
	// 构建系统提示词
	systemPrompt := `你是Kubernetes智能查询助手，可以调用工具来回答用户的问题。
你可以使用以下工具：
1. list_persistent_volumes: 获取所有PV的列表信息
2. get_largest_pv: 获取集群中容量最大的PV
3. find_pods_using_pvc: 查找使用指定PVC的Pod，参数: pvc_name (PVC名称), namespace (命名空间)
4. list_pods_in_namespace: 列出指定命名空间下的Pod，参数: namespace (命名空间，留空表示所有命名空间)
5. get_pod_resource_usage: 获取Pod的资源配置，参数: pod_name (Pod名称), namespace (命名空间)
6. list_namespaces: 列出集群中所有的命名空间

如果需要调用工具，你必须用以下格式返回：
{"name": "工具名称", "arguments": {"参数名": "参数值"}}

如果已经获取到足够的信息，请直接用自然语言回答用户的问题，不要调用不必要的工具。`

	// 定义工具列表（给LLM看的描述）
	functions := []llm.FunctionDefinition{
		{
			Name:        "list_persistent_volumes",
			Description: "获取集群中所有PV的列表信息",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        "get_largest_pv",
			Description: "获取集群中容量最大的PV信息",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        "find_pods_using_pvc",
			Description: "查找使用指定PVC的Pod",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pvc_name": map[string]any{
						"type":        "string",
						"description": "PVC的名称",
					},
					"namespace": map[string]any{
						"type":        "string",
						"description": "PVC所在的命名空间",
					},
				},
				"required": []string{"pvc_name", "namespace"},
			},
		},
		{
			Name:        "list_pods_in_namespace",
			Description: "列出指定命名空间下的所有Pod",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"namespace": map[string]any{
						"type":        "string",
						"description": "命名空间，留空表示查询所有命名空间",
					},
				},
			},
		},
		{
			Name:        "get_pod_resource_usage",
			Description: "获取指定Pod的资源配置信息",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pod_name": map[string]any{
						"type":        "string",
						"description": "Pod的名称",
					},
					"namespace": map[string]any{
						"type":        "string",
						"description": "Pod所在的命名空间",
					},
				},
				"required": []string{"pod_name", "namespace"},
			},
		},
		{
			Name:        "list_namespaces",
			Description: "列出集群中所有的命名空间",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	}

	// 初始化消息历史
	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userQuery},
	}

	// 最多允许5轮工具调用
	maxSteps := 5
	for step := 0; step < maxSteps; step++ {
		// 调用LLM
		resp, err := a.llmClient.ChatCompletion(ctx, messages, functions)
		if err != nil {
			return "", fmt.Errorf("LLM调用失败: %w", err)
		}

		// 尝试解析工具调用
		funcCall, err := parseToolCall(resp)
		if err != nil {
			// 不是工具调用，直接返回内容
			return resp.Content, nil
		}

		// 处理函数调用
		fmt.Printf("第%d步：调用工具 %s\n", step+1, funcCall.Name)
		if len(funcCall.Arguments) > 0 {
			args := make([]string, 0, len(funcCall.Arguments))
			for k, v := range funcCall.Arguments {
				args = append(args, fmt.Sprintf("%s=%v", k, v))
			}
			fmt.Printf("参数：%s\n", strings.Join(args, ", "))
		}

		// 执行工具调用
		result, err := a.executeToolCall(ctx, funcCall.Name, funcCall.Arguments)
		if err != nil {
			return "", fmt.Errorf("调用工具失败: %w", err)
		}

		fmt.Printf("工具返回结果长度：%d 字节\n", len(result))

		// 将工具调用结果添加到消息历史
		messages = append(messages, *resp)
		messages = append(messages, llm.Message{
			Role:    "user", // 大部分模型都支持用user角色返回工具结果
			Content: fmt.Sprintf("工具返回结果：\n%s", result),
		})
	}

	return "", fmt.Errorf("超过最大调用轮次，无法完成查询")
}

// parseToolCall 自动解析各种格式的工具调用
func parseToolCall(resp *llm.Message) (*llm.FunctionCall, error) {
	// 情况1：已经是结构化的FunctionCall（OpenAI原生格式）
	if resp.FunctionCall != nil {
		return resp.FunctionCall, nil
	}

	content := strings.TrimSpace(resp.Content)
	if content == "" {
		return nil, fmt.Errorf("空响应")
	}

	// 情况2：GLM等模型的特殊格式 <|FunctionCallBegin|>...<|FunctionCallEnd|>
	if strings.Contains(content, "<|FunctionCallBegin|>") {
		startIdx := strings.Index(content, "<|FunctionCallBegin|>") + len("<|FunctionCallBegin|>")
		endIdx := strings.Index(content, "<|FunctionCallEnd|>")
		if endIdx > startIdx {
			content = strings.TrimSpace(content[startIdx:endIdx])
		}
	}

	// 情况3：被markdown代码块包裹
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	// 情况4：GLM返回的数组格式 [{"name":"xxx", "parameters":{}}]
	var glmFuncCalls []struct {
		Name       string         `json:"name"`
		Parameters map[string]any `json:"parameters"`
	}
	if err := json.Unmarshal([]byte(content), &glmFuncCalls); err == nil && len(glmFuncCalls) > 0 {
		return &llm.FunctionCall{
			Name:      glmFuncCalls[0].Name,
			Arguments: glmFuncCalls[0].Parameters,
		}, nil
	}

	// 情况5：标准的单个函数调用格式 {"name":"xxx", "arguments":{}}
	var standardCall llm.FunctionCall
	if err := json.Unmarshal([]byte(content), &standardCall); err == nil && standardCall.Name != "" {
		return &standardCall, nil
	}

	// 情况6：参数键是parameters而不是arguments的格式
	var paramCall struct {
		Name       string         `json:"name"`
		Parameters map[string]any `json:"parameters"`
	}
	if err := json.Unmarshal([]byte(content), &paramCall); err == nil && paramCall.Name != "" {
		return &llm.FunctionCall{
			Name:      paramCall.Name,
			Arguments: paramCall.Parameters,
		}, nil
	}

	// 不是工具调用
	return nil, fmt.Errorf("不是有效的工具调用格式")
}

// executeToolCall 执行工具调用
func (a *Agent) executeToolCall(ctx context.Context, funcName string, args map[string]any) (string, error) {
	// 工具名格式转换，支持下划线命名和驼峰命名
	funcName = snakeToPascalCase(funcName)

	// 使用反射调用对应的方法
	method := reflect.ValueOf(a.tools).MethodByName(funcName)
	if !method.IsValid() {
		return "", fmt.Errorf("未知工具: %s", funcName)
	}

	// 构建参数
	var callArgs []reflect.Value
	callArgs = append(callArgs, reflect.ValueOf(ctx))

	// 获取方法的参数类型
	methodType := method.Type()
	numParams := methodType.NumIn() - 1 // 减去ctx参数

	if numParams != len(args) {
		return "", fmt.Errorf("工具 %s 需要 %d 个参数，提供了 %d 个", funcName, numParams, len(args))
	}

	// 按顺序添加参数
	switch numParams {
	case 2:
		// 两个参数的函数，顺序是: pvc_name, namespace 或 pod_name, namespace
		pvcName, pvcOk := args["pvc_name"].(string)
		podName, podOk := args["pod_name"].(string)
		namespace, nsOk := args["namespace"].(string)

		if !nsOk {
			return "", fmt.Errorf("缺少namespace参数")
		}

		if pvcOk {
			callArgs = append(callArgs, reflect.ValueOf(pvcName))
			callArgs = append(callArgs, reflect.ValueOf(namespace))
		} else if podOk {
			callArgs = append(callArgs, reflect.ValueOf(podName))
			callArgs = append(callArgs, reflect.ValueOf(namespace))
		} else {
			return "", fmt.Errorf("缺少pvc_name或pod_name参数")
		}
	case 1:
		// 一个参数的函数
		namespace, ok := args["namespace"].(string)
		if ok {
			callArgs = append(callArgs, reflect.ValueOf(namespace))
		}
	}

	// 调用方法
	results := method.Call(callArgs)

	// 处理返回值
	if len(results) != 2 {
		return "", fmt.Errorf("工具 %s 返回值格式错误", funcName)
	}

	resultStr, ok := results[0].Interface().(string)
	if !ok {
		return "", fmt.Errorf("工具 %s 返回值不是字符串", funcName)
	}

	err, ok := results[1].Interface().(error)
	if ok && err != nil {
		return "", err
	}

	return resultStr, nil
}

// snakeToPascalCase 将下划线命名转换为帕斯卡命名
func snakeToPascalCase(s string) string {
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.Title(s)
	s = strings.ReplaceAll(s, " ", "")
	return s
}
