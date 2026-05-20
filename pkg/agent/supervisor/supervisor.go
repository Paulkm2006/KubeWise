package supervisor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kubewise/kubewise/pkg/llm"
)

// Decision represents the supervisor's verdict on the agent's state.
type Decision string

const (
	DecisionContinue Decision = "continue" // Agent is making progress; grant more steps.
	DecisionReset    Decision = "reset"    // Agent is stuck; inject hint and restart loop.
	DecisionAbort    Decision = "abort"    // Agent is hopelessly stuck; give up.
)

// Result is what the supervisor returns after evaluating the agent's state.
type Result struct {
	Decision    Decision
	ExtraSteps  int    // Only when DecisionContinue: additional steps to grant.
	Hint        string // Only when DecisionReset: guidance injected as a user message.
	Explanation string // Human-readable reason for the decision.
}

// Config holds tunable parameters for the supervisor.
type Config struct {
	Enabled            bool `mapstructure:"enabled"`
	RepeatThreshold    int  `mapstructure:"repeat_threshold"`
	PingPongThreshold  int  `mapstructure:"ping_pong_threshold"`
	SameToolThreshold  int  `mapstructure:"same_tool_threshold"`
	MaxExtensions      int  `mapstructure:"max_extensions"`
	ExtensionStepGrant int  `mapstructure:"extension_step_grant"`
	MaxEvaluatorCalls  int  `mapstructure:"max_evaluator_calls"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Enabled:            true,
		RepeatThreshold:    3,
		PingPongThreshold:  3,
		SameToolThreshold:  5,
		MaxExtensions:      2,
		ExtensionStepGrant: 10,
		MaxEvaluatorCalls:  2,
	}
}

// Supervisor watches an agent's tool-calling loop and intervenes when
// it detects a stuck pattern or the agent reaches maxSteps.
type Supervisor struct {
	config    Config
	llmClient *llm.Client
	detector  *loopDetector

	// Per-query mutable state
	extensionsGranted  int
	evaluatorCallsUsed int
}

// New creates a new Supervisor. If cfg.Enabled is false, all methods are no-ops.
func New(cfg Config, llmClient *llm.Client) *Supervisor {
	if cfg.RepeatThreshold <= 0 {
		cfg.RepeatThreshold = 3
	}
	if cfg.PingPongThreshold <= 0 {
		cfg.PingPongThreshold = 3
	}
	if cfg.SameToolThreshold <= 0 {
		cfg.SameToolThreshold = 5
	}
	if cfg.MaxExtensions <= 0 {
		cfg.MaxExtensions = 2
	}
	if cfg.ExtensionStepGrant <= 0 {
		cfg.ExtensionStepGrant = 10
	}
	if cfg.MaxEvaluatorCalls <= 0 {
		cfg.MaxEvaluatorCalls = 2
	}
	return &Supervisor{
		config:    cfg,
		llmClient: llmClient,
		detector:  newLoopDetector(20),
	}
}

// Enabled returns whether the supervisor is active.
func (s *Supervisor) Enabled() bool {
	return s != nil && s.config.Enabled
}

// Reset clears per-query state. Call at the start of each HandleQuery.
func (s *Supervisor) Reset() {
	if s == nil {
		return
	}
	s.extensionsGranted = 0
	s.evaluatorCallsUsed = 0
	s.detector.reset()
}

// CheckLoop runs cheap local loop detection after tool execution.
// Returns (triggered=true, result) if a loop pattern is detected.
// When triggered, the result already contains the LLM-based evaluation.
func (s *Supervisor) CheckLoop(ctx context.Context, step int, toolCalls []llm.ToolCall, messages []llm.Message) (bool, *Result) {
	if !s.Enabled() {
		return false, nil
	}

	// Record fingerprints for this step
	var names []string
	var args []map[string]any
	for _, tc := range toolCalls {
		names = append(names, tc.Function.Name)
		args = append(args, tc.Function.Arguments)
	}
	fps := buildFingerprints(names, args)
	s.detector.record(fps)

	// Run local detection
	desc := s.detector.detect(s.config)
	if desc == "" {
		return false, nil
	}

	// Loop detected — run LLM evaluation
	result, err := s.evaluateProgress(ctx, messages, desc)
	if err != nil {
		// Fallback: abort with the loop description
		return true, &Result{
			Decision:    DecisionAbort,
			Explanation: fmt.Sprintf("loop detected (%s) and evaluation failed: %v", desc, err),
		}
	}
	return true, result
}

// Evaluate runs an LLM-based progress evaluation. Called when maxSteps is reached.
func (s *Supervisor) Evaluate(ctx context.Context, messages []llm.Message, currentStep, maxSteps int) (*Result, error) {
	if !s.Enabled() {
		return &Result{Decision: DecisionAbort, Explanation: fmt.Sprintf("超过最大调用轮次（%d）", maxSteps)}, nil
	}

	if s.evaluatorCallsUsed >= s.config.MaxEvaluatorCalls {
		return &Result{
			Decision:    DecisionAbort,
			Explanation: fmt.Sprintf("超过最大调用轮次（%d）且已用尽supervisor评估次数", maxSteps),
		}, nil
	}

	return s.evaluateProgress(ctx, messages, fmt.Sprintf("reached max steps (%d)", maxSteps))
}

// extractToolHistory builds fingerprint + result slices from the message history.
func extractToolHistory(messages []llm.Message) ([]toolCallFingerprint, []string) {
	var fps []toolCallFingerprint
	var results []string

	for i, msg := range messages {
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				fps = append(fps, toolCallFingerprint{
					toolName: tc.Function.Name,
					argsKey:  stableJSON(tc.Function.Arguments),
				})
			}
		}
		if msg.Role == "tool" {
			content := msg.Content
			// Strip the "工具返回结果：\n" prefix if present
			if strings.HasPrefix(content, "工具返回结果：\n") {
				content = strings.TrimPrefix(content, "工具返回结果：\n")
			} else if strings.HasPrefix(content, "工具返回结果：") {
				content = strings.TrimPrefix(content, "工具返回结果：")
			}
			results = append(results, content)
		}
		// Also handle the operation agent's inline format
		_ = i
	}

	return fps, results
}

// evaluateProgress calls the LLM to evaluate whether the agent is making progress.
func (s *Supervisor) evaluateProgress(ctx context.Context, messages []llm.Message, reason string) (*Result, error) {
	s.evaluatorCallsUsed++

	if s.llmClient == nil {
		return nil, fmt.Errorf("no LLM client available for supervisor evaluation")
	}

	fps, results := extractToolHistory(messages)
	summary := buildConversationSummary(fps, results)

	// Truncate very long summaries
	if len(summary) > 2000 {
		summary = summary[:2000] + "\n...(truncated)"
	}

	systemPrompt := `You are a supervisor evaluating whether an AI agent is making progress toward answering a user's Kubernetes query.

The agent has been calling tools in a loop. Below is a summary of its tool-call history.

Decide:
- "continue": The agent is making genuine progress (each step provides new information toward the goal). Return the number of additional steps needed (5-15).
- "reset": The agent is stuck in a repetitive loop. Provide a short hint (1-2 sentences) about what it should do differently.
- "abort": The agent is hopelessly stuck and cannot recover. Explain why.

Return JSON only: {"decision": "continue"|"reset"|"abort", "extra_steps": <int>, "hint": "<string>", "explanation": "<string>"}`

	userMsg := fmt.Sprintf("Reason for evaluation: %s\n\nTool-call history:\n%s", reason, summary)

	evalMessages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMsg},
	}

	resp, err := s.llmClient.ChatCompletion(ctx, evalMessages, nil)
	if err != nil {
		return nil, fmt.Errorf("supervisor LLM call failed: %w", err)
	}

	content := strings.TrimSpace(resp.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var evalResp struct {
		Decision   string `json:"decision"`
		ExtraSteps int    `json:"extra_steps"`
		Hint       string `json:"hint"`
		Explanation string `json:"explanation"`
	}
	if err := json.Unmarshal([]byte(content), &evalResp); err != nil {
		return nil, fmt.Errorf("supervisor response parse failed: %w (raw: %s)", err, content)
	}

	result := &Result{
		Decision:    Decision(evalResp.Decision),
		ExtraSteps:  evalResp.ExtraSteps,
		Hint:        evalResp.Hint,
		Explanation: evalResp.Explanation,
	}

	// Validate and constrain
	switch result.Decision {
	case DecisionContinue:
		if s.extensionsGranted >= s.config.MaxExtensions {
			result.Decision = DecisionAbort
			result.Explanation = fmt.Sprintf("agent making progress but max extensions (%d) reached. %s", s.config.MaxExtensions, result.Explanation)
			return result, nil
		}
		if result.ExtraSteps <= 0 {
			result.ExtraSteps = s.config.ExtensionStepGrant
		}
		if result.ExtraSteps > 20 {
			result.ExtraSteps = 20
		}
		s.extensionsGranted++
	case DecisionReset:
		if result.Hint == "" {
			result.Hint = "你已经陷入循环，请换一种方式来获取信息，直接用已有的结果回答用户的问题。"
		}
	default:
		result.Decision = DecisionAbort
	}

	return result, nil
}
