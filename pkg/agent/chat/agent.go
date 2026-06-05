package chat

import (
	"context"
	"fmt"
	"time"

	"github.com/kubewise/kubewise/pkg/llm"
	"github.com/kubewise/kubewise/pkg/stream"
	"github.com/kubewise/kubewise/pkg/types"
	"go.uber.org/zap"
)

const (
	Name        = "kubewise_chat_agent"
	Description = "General conversation and knowledge-sharing agent for non-operational questions."
)

// Agent handles normal chat, knowledge explanation, and low-confidence fallback.
type Agent struct {
	llmClient *llm.Client
	eventCh   chan<- stream.Event
	queryID   string
	log       *zap.Logger
}

// New creates a chat agent. It intentionally has no Kubernetes tools.
func New(llmClient *llm.Client) *Agent {
	return &Agent{llmClient: llmClient}
}

func (a *Agent) SetLogger(l *zap.Logger) { a.log = l }

func (a *Agent) logger() *zap.Logger {
	if a.log == nil {
		return zap.NewNop()
	}
	return a.log
}

// SetEventChannel sets the event channel and query ID for streaming progress.
func (a *Agent) SetEventChannel(eventCh chan<- stream.Event, queryID string) {
	a.eventCh = eventCh
	a.queryID = queryID
}

func (a *Agent) emit(e stream.Event) {
	if a.eventCh == nil {
		return
	}
	select {
	case a.eventCh <- e:
	default:
	}
}

func (a *Agent) buildInstruction() string {
	return `你是 KubeWise 智能体。

你的主要身份是 Kubernetes 智能运维助手，同时也可以作为通用聊天和知识解释助手。

适用场景：
- 寒暄、普通聊天、介绍自己。
- 解释 Kubernetes、后端、云原生、安全、运维、编程等基础知识。
- 帮助初学者理解概念、对比概念、梳理学习路径。
- 当路由器置信度较低时，作为安全兜底回答。

边界：
- 不要声称已经查询真实集群，除非用户明确要求集群查询且请求已路由到专门的 Kubernetes Agent。
- 不要编造集群资源、Pod 状态、节点状态或安全扫描结果。
- 不要执行或建议已完成任何写操作。
- 如果用户要求真实集群查询、故障排查、安全审计、资源修改或 Helm 部署，说明这类任务应交给 KubeWise 的专业 Agent 处理，并可提示用户重新明确目标。

回答风格：
- 使用中文，直接、清晰、适合初学者。
- 对概念问题优先用类比和小例子解释。
- 对操作类知识可以给示例命令，但要说明只是示例，不代表已经执行。`
}

// HandleQuery answers without tool calls.
func (a *Agent) HandleQuery(ctx context.Context, userQuery string, _ types.Entities) (string, error) {
	start := time.Now()
	var inTokens, outTokens int
	a.emit(stream.AgentStart{AgentName: "Chat Agent", QueryID: a.queryID})
	a.emit(stream.Phase{QueryID: a.queryID, Phase: "chatting"})
	a.logger().Debug("handling chat query", zap.String("query", userQuery))
	defer func() {
		a.emit(stream.AgentDone{
			QueryID:   a.queryID,
			Duration:  time.Since(start),
			InTokens:  inTokens,
			OutTokens: outTokens,
		})
	}()

	messages := []llm.Message{
		{Role: "system", Content: a.buildInstruction()},
		{Role: "user", Content: userQuery},
	}

	resp, err := a.llmClient.ChatCompletion(ctx, messages, nil, func(chunk llm.StreamChunk) {
		if chunk.Content != "" {
			a.emit(stream.LLMTextDelta{QueryID: a.queryID, Delta: chunk.Content})
		}
	})
	if err != nil {
		return "", fmt.Errorf("LLM chat failed: %w", err)
	}
	if resp.Usage != nil {
		inTokens += resp.Usage.PromptTokens
		outTokens += resp.Usage.CompletionTokens
	}
	return resp.Content, nil
}
