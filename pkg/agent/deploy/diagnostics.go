package deploy

import (
	"strings"

	"github.com/kubewise/kubewise/pkg/tui/events"
)

// emitDetected applies format detection to content and emits the appropriate Render*Event.
func (a *Agent) emitDetected(content string) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return
	}

	for line := range strings.SplitSeq(content, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "apiVersion:") || strings.HasPrefix(t, "kind:") {
			a.emit(events.RenderCodeEvent{QueryID: a.queryID, Language: "yaml", Content: content})
			return
		}
	}

	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		a.emit(events.RenderCodeEvent{QueryID: a.queryID, Language: "json", Content: content})
		return
	}

	if headers, rows, ok := parseTable(content); ok {
		a.emit(events.RenderTableEvent{QueryID: a.queryID, Headers: headers, Rows: rows})
		return
	}

	lines := strings.Split(content, "\n")
	statusOf := make(map[int]string)
	matchCount := 0
	for i, line := range lines {
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		var status string
		switch {
		case containsAny(lower, "error", "failed", "crashloopbackoff", "unhealthy", "critical"):
			status = "error"
		case containsAny(lower, "pending", "terminating", "warning"):
			status = "warn"
		case containsAny(lower, "running", "healthy"):
			status = "ok"
		}
		if status != "" {
			statusOf[i] = status
			matchCount++
		}
	}
	if matchCount >= 2 {
		items := make([]events.ListItem, 0)
		for i, line := range lines {
			if line == "" {
				continue
			}
			s, ok := statusOf[i]
			if !ok {
				s = "info"
			}
			items = append(items, events.ListItem{Status: s, Text: line})
		}
		a.emit(events.RenderListEvent{QueryID: a.queryID, Items: items})
		return
	}

	var kvLines []string
	var nonEmptyCount int
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		nonEmptyCount++
		if idx := strings.Index(l, ": "); idx > 0 {
			before := strings.TrimSpace(l[:idx])
			if before != "" && !strings.Contains(before, " ") {
				kvLines = append(kvLines, l)
			}
		}
	}
	if len(kvLines) >= 2 && nonEmptyCount > 0 && len(kvLines)*2 >= nonEmptyCount {
		pairs := make([]events.KVPair, 0, len(kvLines))
		for _, l := range kvLines {
			key, val, _ := strings.Cut(l, ": ")
			pairs = append(pairs, events.KVPair{Key: strings.TrimSpace(key), Value: strings.TrimSpace(val)})
		}
		a.emit(events.RenderKVEvent{QueryID: a.queryID, Pairs: pairs})
		return
	}

	a.emit(events.RenderTextEvent{QueryID: a.queryID, Text: content})
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func parseTable(content string) (headers []string, rows [][]string, ok bool) {
	lines := strings.Split(content, "\n")
	var tableLines []string
	for _, l := range lines {
		if strings.Contains(l, "|") {
			tableLines = append(tableLines, l)
		}
	}
	if len(tableLines) < 3 {
		return nil, nil, false
	}
	for _, l := range tableLines {
		if isSeparatorRow(l) {
			continue
		}
		if len(headers) == 0 {
			for cell := range strings.SplitSeq(l, "|") {
				cell = strings.TrimSpace(cell)
				if cell != "" {
					headers = append(headers, cell)
				}
			}
		} else {
			var row []string
			for cell := range strings.SplitSeq(l, "|") {
				cell = strings.TrimSpace(cell)
				if cell != "" {
					row = append(row, cell)
				}
			}
			if len(row) > 0 {
				rows = append(rows, row)
			}
		}
	}
	return headers, rows, len(headers) > 0 && len(rows) > 0
}

func isSeparatorRow(line string) bool {
	hasCell := false
	for cell := range strings.SplitSeq(line, "|") {
		cell = strings.TrimSpace(cell)
		if cell == "" {
			continue
		}
		hasCell = true
		for _, ch := range cell {
			if ch != '-' && ch != ':' {
				return false
			}
		}
	}
	return hasCell
}
