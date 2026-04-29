package model

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	chromastyles "github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"

	"github.com/kubewise/kubewise/pkg/tui/events"
	tuistyles "github.com/kubewise/kubewise/pkg/tui/styles"
)

// Renderer converts structured event payloads to styled terminal strings.
type Renderer struct {
	width int
}

// NewRenderer creates a Renderer with the given terminal width.
func NewRenderer(width int) *Renderer {
	return &Renderer{width: width}
}

// SetWidth updates the terminal width (called on resize).
func (r *Renderer) SetWidth(w int) { r.width = w }

// RenderText returns plain text, word-wrapped to terminal width.
func (r *Renderer) RenderText(text string) string {
	return lipgloss.NewStyle().Width(r.width).Render(text)
}

// RenderTable renders headers + rows as a lipgloss-bordered table.
func (r *Renderer) RenderTable(headers []string, rows [][]string) string {
	colWidths := make([]int, len(headers))
	for i, h := range headers {
		colWidths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(colWidths) && len(cell) > colWidths[i] {
				colWidths[i] = len(cell)
			}
		}
	}

	sep := strings.Repeat("─", r.width-2)

	var sb strings.Builder
	sb.WriteString(sep + "\n")
	for i, h := range headers {
		sb.WriteString(lipgloss.NewStyle().Width(colWidths[i]+2).Bold(true).Render(h))
	}
	sb.WriteString("\n" + sep + "\n")

	for _, row := range rows {
		for i, cell := range row {
			if i >= len(colWidths) {
				break
			}
			sb.WriteString(lipgloss.NewStyle().Width(colWidths[i]+2).Render(cell))
		}
		sb.WriteString("\n")
	}
	sb.WriteString(sep)
	return sb.String()
}

// RenderCode renders a syntax-highlighted fenced code block.
func (r *Renderer) RenderCode(language, content string) string {
	lexer := lexers.Get(language)
	if lexer == nil {
		lexer = lexers.Fallback
	}
	style := chromastyles.Get("monokai")
	if style == nil {
		style = chromastyles.Fallback
	}
	formatter := formatters.Get("terminal256")
	if formatter == nil {
		header := tuistyles.CodeLangStyle.Render(language)
		return tuistyles.CodeBlockStyle.Width(r.width - 2).Render(header + "\n" + content)
	}

	tokenizer, err := lexer.Tokenise(nil, content)
	if err != nil {
		return content
	}
	var buf bytes.Buffer
	if err := formatter.Format(&buf, style, tokenizer); err != nil {
		return content
	}

	header := tuistyles.CodeLangStyle.Render(language)
	return tuistyles.CodeBlockStyle.Width(r.width-2).Render(header + "\n" + buf.String())
}

// RenderKV renders key-value pairs with right-aligned keys.
func (r *Renderer) RenderKV(pairs []events.KVPair) string {
	maxKey := 0
	for _, p := range pairs {
		if len(p.Key) > maxKey {
			maxKey = len(p.Key)
		}
	}

	var sb strings.Builder
	for _, p := range pairs {
		key := tuistyles.KVKeyStyle.Width(maxKey + 2).Align(lipgloss.Right).Render(p.Key)
		sb.WriteString(fmt.Sprintf("%s  %s\n", key, p.Value))
	}
	return strings.TrimRight(sb.String(), "\n")
}

// RenderList renders status items with coloured status icons.
func (r *Renderer) RenderList(items []events.ListItem) string {
	var sb strings.Builder
	for _, item := range items {
		icon, style := statusIconStyle(item.Status)
		sb.WriteString(style.Render(icon+" "+item.Text) + "\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

func statusIconStyle(status string) (string, lipgloss.Style) {
	switch status {
	case "ok":
		return "✓", tuistyles.ListOKStyle
	case "warn":
		return "⚠", tuistyles.ListWarnStyle
	case "error":
		return "✗", tuistyles.ListErrorStyle
	default:
		return "•", tuistyles.ListInfoStyle
	}
}
