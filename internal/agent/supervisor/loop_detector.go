package supervisor

import (
	"encoding/json"
	"fmt"
	"strings"
)

// toolCallFingerprint uniquely identifies a tool call by name + args.
type toolCallFingerprint struct {
	toolName string
	argsKey  string
}

// String returns a human-readable representation for logging.
func (f toolCallFingerprint) String() string {
	if len(f.argsKey) > 80 {
		return fmt.Sprintf("%s(%s...)", f.toolName, f.argsKey[:80])
	}
	return fmt.Sprintf("%s(%s)", f.toolName, f.argsKey)
}

// stableJSON produces a deterministic JSON string from map[string]any.
// Go's json.Marshal sorts map keys alphabetically since Go 1.12, so this
// is sufficient for fingerprinting.
func stableJSON(args map[string]any) string {
	if len(args) == 0 {
		return "{}"
	}
	b, err := json.Marshal(args)
	if err != nil {
		// Fallback: use fmt.Sprintf which also produces deterministic output for maps
		return fmt.Sprintf("%v", args)
	}
	return string(b)
}

// loopDetector tracks recent tool calls and detects repetitive patterns.
type loopDetector struct {
	history []toolCallFingerprint // sliding window of recent fingerprints
	maxLen  int                   // max window size
}

// newLoopDetector creates a detector with the given window size.
func newLoopDetector(windowSize int) *loopDetector {
	if windowSize <= 0 {
		windowSize = 20
	}
	return &loopDetector{
		history: make([]toolCallFingerprint, 0, windowSize),
		maxLen:  windowSize,
	}
}

// record appends fingerprints for the given tool calls and trims the window.
func (d *loopDetector) record(fingerprints []toolCallFingerprint) {
	d.history = append(d.history, fingerprints...)
	if len(d.history) > d.maxLen {
		d.history = d.history[len(d.history)-d.maxLen:]
	}
}

// reset clears the detector's history.
func (d *loopDetector) reset() {
	d.history = d.history[:0]
}

// detect checks the history for loop patterns and returns a description
// of the detected pattern, or empty string if none found.
func (d *loopDetector) detect(cfg Config) string {
	n := len(d.history)
	if n < 2 {
		return ""
	}

	if desc := d.detectExactRepeat(cfg.RepeatThreshold); desc != "" {
		return desc
	}
	if desc := d.detectPingPong(cfg.PingPongThreshold); desc != "" {
		return desc
	}
	if desc := d.detectSameToolHammer(cfg.SameToolThreshold); desc != "" {
		return desc
	}
	return ""
}

// detectExactRepeat checks if the same fingerprint appears N consecutive times at the end.
func (d *loopDetector) detectExactRepeat(threshold int) string {
	if threshold <= 0 || len(d.history) < threshold {
		return ""
	}

	last := d.history[len(d.history)-1]
	count := 1
	for i := len(d.history) - 2; i >= 0; i-- {
		if d.history[i] == last {
			count++
		} else {
			break
		}
	}

	if count >= threshold {
		return fmt.Sprintf("exact repeat: %s called %d times consecutively", last.toolName, count)
	}
	return ""
}

// detectPingPong checks for A-B-A-B alternation pattern.
func (d *loopDetector) detectPingPong(threshold int) string {
	if threshold <= 0 || len(d.history) < threshold*2 {
		return ""
	}

	n := len(d.history)
	a := d.history[n-2]
	b := d.history[n-1]
	if a == b {
		return "" // not alternating, that's exact repeat
	}

	cycles := 1 // we have one A-B pair at the end
	for i := n - 3; i >= 1; i-- {
		if d.history[i] == b && d.history[i-1] == a {
			cycles++
			i-- // skip the matched A
		} else {
			break
		}
	}

	if cycles >= threshold {
		return fmt.Sprintf("ping-pong: alternating between %s and %s (%d cycles)", a.toolName, b.toolName, cycles)
	}
	return ""
}

// detectSameToolHammer checks if the same tool name (ignoring args) is called
// N consecutive times.
func (d *loopDetector) detectSameToolHammer(threshold int) string {
	if threshold <= 0 || len(d.history) < threshold {
		return ""
	}

	lastName := d.history[len(d.history)-1].toolName
	count := 1
	for i := len(d.history) - 2; i >= 0; i-- {
		if d.history[i].toolName == lastName {
			count++
		} else {
			break
		}
	}

	if count >= threshold {
		// Collect distinct args for context
		var distinctArgs []string
		seen := make(map[string]bool)
		for i := len(d.history) - count; i < len(d.history); i++ {
			key := d.history[i].argsKey
			if !seen[key] {
				seen[key] = true
				distinctArgs = append(distinctArgs, key)
			}
		}
		return fmt.Sprintf("same-tool hammer: %s called %d times consecutively (%d distinct args)", lastName, count, len(distinctArgs))
	}
	return ""
}

// buildFingerprints creates fingerprints from tool calls.
func buildFingerprints(toolNames []string, toolArgs []map[string]any) []toolCallFingerprint {
	fps := make([]toolCallFingerprint, len(toolNames))
	for i, name := range toolNames {
		fps[i] = toolCallFingerprint{
			toolName: name,
			argsKey:  stableJSON(toolArgs[i]),
		}
	}
	return fps
}

// buildConversationSummary creates a compact summary of tool-call history from messages.
func buildConversationSummary(messages []toolCallFingerprint, toolResults []string) string {
	var sb strings.Builder
	step := 0
	for i, fp := range messages {
		step++
		result := ""
		if i < len(toolResults) {
			result = toolResults[i]
			// Truncate long results
			if len(result) > 100 {
				result = result[:100] + "..."
			}
			// Replace newlines for single-line display
			result = strings.ReplaceAll(result, "\n", " ")
		}
		if result != "" {
			fmt.Fprintf(&sb, "Step %d: %s → %s\n", step, fp.String(), result)
		} else {
			fmt.Fprintf(&sb, "Step %d: %s\n", step, fp.String())
		}
	}
	return sb.String()
}
