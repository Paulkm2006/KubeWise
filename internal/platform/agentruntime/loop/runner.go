package loop

import (
	"context"
	"fmt"
	"time"

	"github.com/kubewise/kubewise/internal/platform/agentruntime/event"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/supervisor"
	toolv2 "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/v2"
	"github.com/kubewise/kubewise/internal/utils/llm"
)

type EmitFunc func(event.Event)

type TerminalHandler func(ctx context.Context, call llm.ToolCall, messages []llm.Message) (TerminalResult, error)

type TerminalResult struct {
	Done        bool
	Content     string
	Data        any
	ToolMessage string
}

type RepeatDetector func(step int, calls []llm.ToolCall) error

type Config struct {
	QueryID        string
	AgentName      string
	LLM            llm.ClientPort
	Messages       []llm.Message
	Tools          []llm.FunctionDefinition
	Executor       *toolv2.Executor
	Policy         toolv2.ExecutePolicy
	MaxSteps       int
	Emit           EmitFunc
	ToolPrefix     string
	TerminalTools  map[string]TerminalHandler
	Supervisor     *supervisor.Supervisor
	RepeatCheck    RepeatDetector
	ContinueOnText bool
}

type Result struct {
	Content      string
	TerminalData any
	InTokens     int
	OutTokens    int
	Messages     []llm.Message
}

func Run(ctx context.Context, cfg Config) (Result, error) {
	if cfg.LLM == nil {
		return Result{}, fmt.Errorf("llm unavailable")
	}
	if cfg.MaxSteps <= 0 {
		cfg.MaxSteps = 20
	}
	if cfg.Supervisor != nil {
		cfg.Supervisor.Reset()
	}
	messages := append([]llm.Message(nil), cfg.Messages...)
	var out Result
	stepsRemaining := cfg.MaxSteps
	for step := 0; step < stepsRemaining; step++ {
		emit(cfg.Emit, event.Phase{QueryID: cfg.QueryID, Phase: "thinking"})
		resp, err := cfg.LLM.Complete(ctx, llm.CompletionRequest{
			Messages: messages,
			Tools:    cfg.Tools,
			OnEvent: func(ev llm.StreamEvent) {
				if ev.Content != "" {
					emit(cfg.Emit, event.LLMTextDelta{QueryID: cfg.QueryID, Delta: ev.Content})
				}
			},
		})
		if err != nil {
			return out, err
		}
		if resp.Usage != nil {
			out.InTokens += resp.Usage.PromptTokens
			out.OutTokens += resp.Usage.CompletionTokens
		}
		msg := resp.Message
		if len(msg.ToolCalls) == 0 {
			if cfg.ContinueOnText {
				messages = append(messages, msg)
				continue
			}
			out.Content = msg.Content
			out.Messages = append(messages, msg)
			return out, nil
		}
		messages = append(messages, msg)
		for _, tc := range msg.ToolCalls {
			toolName := tc.Function.Name
			if handler := cfg.TerminalTools[toolName]; handler != nil {
				terminal, err := handler(ctx, tc, messages)
				if err != nil {
					messages = append(messages, llm.Message{
						Role:       "tool",
						Content:    err.Error(),
						ToolCallID: tc.ID,
					})
					continue
				}
				if terminal.ToolMessage != "" {
					messages = append(messages, llm.Message{
						Role:       "tool",
						Content:    terminal.ToolMessage,
						ToolCallID: tc.ID,
					})
				}
				if terminal.Done {
					out.Content = terminal.Content
					out.TerminalData = terminal.Data
					out.Messages = messages
					return out, nil
				}
				continue
			}
			if cfg.Executor == nil {
				return out, fmt.Errorf("tool executor unavailable for %s", toolName)
			}
			emit(cfg.Emit, event.Phase{QueryID: cfg.QueryID, Phase: fmt.Sprintf("running tool: %s", toolName)})
			emit(cfg.Emit, event.ToolCall{QueryID: cfg.QueryID, ToolName: toolName, Step: step + 1})
			start := time.Now()
			result, err := cfg.Executor.Execute(ctx, toolName, tc.Function.Arguments, cfg.Policy)
			if err != nil {
				result.Display = fmt.Sprintf("工具调用失败：%v\n请修正参数后重新调用工具。", err)
			}
			emit(cfg.Emit, event.ToolDone{QueryID: cfg.QueryID, ToolName: toolName, Step: step + 1, Elapsed: time.Since(start), Payload: &event.Payload{
				Kind: "tool_result",
				Data: result,
			}})
			prefix := cfg.ToolPrefix
			if prefix == "" {
				prefix = "工具返回结果："
			}
			messages = append(messages, llm.Message{
				Role:       "tool",
				Content:    fmt.Sprintf("%s\n%s", prefix, toolv2.ToolResultToLLMMessage(result)),
				ToolCallID: tc.ID,
			})
		}
		if cfg.RepeatCheck != nil {
			if err := cfg.RepeatCheck(step, msg.ToolCalls); err != nil {
				out.Messages = messages
				return out, err
			}
		}
		if cfg.Supervisor != nil {
			if triggered, loopResult := cfg.Supervisor.CheckLoop(ctx, step, msg.ToolCalls, messages); triggered {
				emit(cfg.Emit, event.Supervisor{
					QueryID:  cfg.QueryID,
					Reason:   "loop detected",
					Decision: string(loopResult.Decision),
					Detail:   loopResult.Explanation,
				})
				switch loopResult.Decision {
				case supervisor.DecisionContinue:
					stepsRemaining += loopResult.ExtraSteps
				case supervisor.DecisionReset:
					messages = append(messages, llm.Message{Role: "user", Content: loopResult.Hint})
					cfg.Supervisor.Reset()
				case supervisor.DecisionAbort:
					out.Messages = messages
					return out, fmt.Errorf("supervisor: %s", loopResult.Explanation)
				}
			}
		}
	}
	out.Messages = messages
	if cfg.Supervisor != nil {
		result, err := cfg.Supervisor.Evaluate(ctx, messages, stepsRemaining, cfg.MaxSteps)
		if err == nil {
			emit(cfg.Emit, event.Supervisor{
				QueryID:  cfg.QueryID,
				Reason:   "max steps reached",
				Decision: string(result.Decision),
				Detail:   result.Explanation,
			})
		}
		if err == nil && result.Decision == supervisor.DecisionContinue {
			cfg.MaxSteps += result.ExtraSteps
			return Run(ctx, Config{
				QueryID:        cfg.QueryID,
				AgentName:      cfg.AgentName,
				LLM:            cfg.LLM,
				Messages:       messages,
				Tools:          cfg.Tools,
				Executor:       cfg.Executor,
				Policy:         cfg.Policy,
				MaxSteps:       result.ExtraSteps,
				Emit:           cfg.Emit,
				ToolPrefix:     cfg.ToolPrefix,
				TerminalTools:  cfg.TerminalTools,
				Supervisor:     cfg.Supervisor,
				RepeatCheck:    cfg.RepeatCheck,
				ContinueOnText: cfg.ContinueOnText,
			})
		}
		if err == nil && result.Decision == supervisor.DecisionReset {
			messages = append(messages, llm.Message{Role: "user", Content: result.Hint})
		}
	}
	return out, fmt.Errorf("max tool-calling steps reached: %d", cfg.MaxSteps)
}

func emit(fn EmitFunc, ev event.Event) {
	if fn != nil {
		fn(ev)
	}
}
