package types

import (
	"encoding/json"
	"fmt"
)

// TaskType 任务类型
type TaskType string

const (
	TaskTypeOperation      TaskType = "operation"      // 操作类
	TaskTypeQuery          TaskType = "query"          // 查询类
	TaskTypeTroubleshooting TaskType = "troubleshooting" // 故障排查类
	TaskTypeSecurity       TaskType = "security"       // 安全审计类
)

// StringSlice 自定义类型，兼容字符串和字符串数组的JSON解析
type StringSlice []string

// UnmarshalJSON 实现自定义JSON解析逻辑
func (s *StringSlice) UnmarshalJSON(data []byte) error {
	// 尝试解析为字符串数组
	var slice []string
	if err := json.Unmarshal(data, &slice); err == nil {
		*s = slice
		return nil
	}

	// 尝试解析为单个字符串
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		if str == "" {
			*s = []string{}
		} else {
			*s = []string{str}
		}
		return nil
	}

	return fmt.Errorf("无法解析为字符串或字符串数组: %s", string(data))
}

// Entities 提取的关键实体
type Entities struct {
	Namespace    string      `json:"namespace,omitempty"`
	ResourceName string      `json:"resource_name,omitempty"`
	ResourceType StringSlice `json:"resource_type,omitempty"`
	AppName      string      `json:"app_name,omitempty"`
	Operation    string      `json:"operation,omitempty"`
}

// IntentClassification 意图分类结果
type IntentClassification struct {
	TaskType            TaskType `json:"task_type"`
	TaskTypeDescription string   `json:"task_type_description"`
	Entities            Entities `json:"entities"`
	Confidence          float64  `json:"confidence"`
}
