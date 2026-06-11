package operation

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
)

// ConfirmationHandler abstracts the per-step confirmation interaction.
// Implementations: StdinConfirmationHandler (CLI), ChannelConfirmationHandler (TUI/API).
type ConfirmationHandler interface {
	// Confirm presents a step to the user and waits for their decision.
	// Returns confirmed=true to execute, non-empty correction to replan,
	// or both false/empty to skip the step.
	Confirm(ctx context.Context, step OperationStep, totalSteps int) (confirmed bool, correction string, err error)
}

// StdinConfirmationHandler is the default CLI implementation.
type StdinConfirmationHandler struct {
	reader *bufio.Reader
	writer io.Writer
}

// NewStdinConfirmationHandler creates a handler that reads from os.Stdin.
func NewStdinConfirmationHandler() *StdinConfirmationHandler {
	return &StdinConfirmationHandler{
		reader: bufio.NewReader(os.Stdin),
		writer: os.Stdout,
	}
}

// Confirm displays the step and prompts for confirmation via stdin.
func (h *StdinConfirmationHandler) Confirm(ctx context.Context, step OperationStep, totalSteps int) (bool, string, error) {
	h.formatStep(step, totalSteps)
	fmt.Fprint(h.writer, "确认执行？[y/N]：")

	line, err := h.reader.ReadString('\n')
	if err != nil {
		return false, "", err
	}
	if strings.ToLower(strings.TrimSpace(line)) == "y" {
		return true, "", nil
	}

	fmt.Fprint(h.writer, "请输入修正指令（直接回车跳过该步骤）：")
	line, err = h.reader.ReadString('\n')
	if err != nil {
		return false, "", err
	}
	return false, strings.TrimSpace(line), nil
}

func (h *StdinConfirmationHandler) formatStep(step OperationStep, totalSteps int) {
	fmt.Fprintf(h.writer, "\n步骤 %d/%d：%s\n", step.StepIndex, totalSteps, operationTypeDisplay(step.OperationType))
	if step.Namespace != "" {
		fmt.Fprintf(h.writer, "  资源：%s/%s (namespace: %s)\n", step.ResourceKind, step.ResourceName, step.Namespace)
	} else {
		fmt.Fprintf(h.writer, "  资源：%s/%s\n", step.ResourceKind, step.ResourceName)
	}
	switch step.OperationType {
	case "scale":
		if step.Replicas != nil {
			fmt.Fprintf(h.writer, "  变更：replicas → %d\n", *step.Replicas)
		} else {
			fmt.Fprintln(h.writer, "  变更：replicas → (未指定)")
		}
	case "restart":
		fmt.Fprintln(h.writer, "  操作：触发滚动重启")
	case "delete":
		fmt.Fprintln(h.writer, "  操作：删除资源（不可撤销）")
	case "apply":
		fmt.Fprintf(h.writer, "  以下 YAML 将被 Apply：\n---\n%s\n---\n", step.GeneratedYAML)
	case "cordon_drain":
		fmt.Fprintf(h.writer, "  操作：%s\n", step.Action)
	case "label_annotate":
		if len(step.Labels) > 0 {
			fmt.Fprintf(h.writer, "  Labels：%v\n", step.Labels)
		}
		if len(step.Annotations) > 0 {
			fmt.Fprintf(h.writer, "  Annotations：%v\n", step.Annotations)
		}
	}
	fmt.Fprintf(h.writer, "  说明：%s\n", step.Description)
}

// ConfirmRequest is sent by the agent to the TUI/API layer.
type ConfirmRequest struct {
	Step       OperationStep
	TotalSteps int
}

// ConfirmResponse is sent back by the TUI/API layer to the agent.
type ConfirmResponse struct {
	Confirmed  bool
	Correction string
	Err        error
}

// ChannelConfirmationHandler enables TUI/API-driven confirmation via channels.
type ChannelConfirmationHandler struct {
	Requests  chan ConfirmRequest
	Responses chan ConfirmResponse
}

// NewChannelConfirmationHandler creates a ChannelConfirmationHandler with unbuffered channels.
func NewChannelConfirmationHandler() *ChannelConfirmationHandler {
	return &ChannelConfirmationHandler{
		Requests:  make(chan ConfirmRequest),
		Responses: make(chan ConfirmResponse),
	}
}

// Confirm sends the step to Requests channel and waits for a response on Responses channel.
func (h *ChannelConfirmationHandler) Confirm(ctx context.Context, step OperationStep, totalSteps int) (bool, string, error) {
	select {
	case h.Requests <- ConfirmRequest{Step: step, TotalSteps: totalSteps}:
	case <-ctx.Done():
		return false, "", ctx.Err()
	}
	select {
	case resp := <-h.Responses:
		return resp.Confirmed, resp.Correction, resp.Err
	case <-ctx.Done():
		return false, "", ctx.Err()
	}
}
