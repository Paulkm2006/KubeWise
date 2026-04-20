package types

// TaskType 任务类型
type TaskType string

const (
	TaskTypeOperation      TaskType = "operation"      // 操作类
	TaskTypeQuery          TaskType = "query"          // 查询类
	TaskTypeTroubleshooting TaskType = "troubleshooting" // 故障排查类
	TaskTypeSecurity       TaskType = "security"       // 安全审计类
)

// Entities 提取的关键实体
type Entities struct {
	Namespace    string `json:"namespace,omitempty"`
	ResourceName string `json:"resource_name,omitempty"`
	ResourceType string `json:"resource_type,omitempty"`
	AppName      string `json:"app_name,omitempty"`
	Operation    string `json:"operation,omitempty"`
}

// IntentClassification 意图分类结果
type IntentClassification struct {
	TaskType            TaskType `json:"task_type"`
	TaskTypeDescription string   `json:"task_type_description"`
	Entities            Entities `json:"entities"`
	Confidence          float64  `json:"confidence"`
}
