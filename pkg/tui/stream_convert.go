package tui

import (
	"github.com/kubewise/kubewise/pkg/stream"
	"github.com/kubewise/kubewise/pkg/tui/events"
)

// ToTUI maps a stream event to a TUI event for the chat progress card and reply blocks.
// Returns false for InteractionRequest, StreamDone, and StreamErr (handled in dispatchStreamEvent).
func ToTUI(ev stream.Event) (events.TUIEvent, bool) {
	switch e := ev.(type) {
	case stream.Phase:
		return events.PhaseEvent{QueryID: e.QueryID, Phase: e.Phase}, true
	case stream.AgentStart:
		return events.AgentStartEvent{QueryID: e.QueryID, AgentName: e.AgentName}, true
	case stream.AgentDone:
		return events.AgentDoneEvent{
			QueryID: e.QueryID, Duration: e.Duration, InTokens: e.InTokens, OutTokens: e.OutTokens,
		}, true
	case stream.ToolCall:
		return events.ToolCallEvent{QueryID: e.QueryID, ToolName: e.ToolName, Step: e.Step}, true
	case stream.ToolDone:
		return events.ToolDoneEvent{
			QueryID: e.QueryID, ToolName: e.ToolName, Step: e.Step, Elapsed: e.Elapsed,
		}, true
	case stream.ToolFail:
		return events.ToolFailEvent{
			QueryID: e.QueryID, ToolName: e.ToolName, Step: e.Step, Elapsed: e.Elapsed,
		}, true
	case stream.RenderText:
		return events.RenderTextEvent{QueryID: e.QueryID, Text: e.Text}, true
	case stream.RenderTable:
		return events.RenderTableEvent{QueryID: e.QueryID, Headers: e.Headers, Rows: e.Rows}, true
	case stream.RenderCode:
		return events.RenderCodeEvent{QueryID: e.QueryID, Language: e.Language, Content: e.Content}, true
	case stream.RenderKV:
		return events.RenderKVEvent{QueryID: e.QueryID, Pairs: copyKVPairs(e.Pairs)}, true
	case stream.RenderList:
		return events.RenderListEvent{QueryID: e.QueryID, Items: copyListItems(e.Items)}, true
	case stream.RenderDetail:
		return events.RenderDetailEvent{QueryID: e.QueryID, Detail: copyResourceDetail(e.Detail)}, true
	case stream.Supervisor:
		return events.SupervisorEvent{
			QueryID: e.QueryID, Reason: e.Reason, Decision: e.Decision, Detail: e.Detail,
		}, true
	default:
		return nil, false
	}
}

func copyKVPairs(in []stream.KVPair) []events.KVPair {
	out := make([]events.KVPair, len(in))
	for i, p := range in {
		out[i] = events.KVPair{Key: p.Key, Value: p.Value}
	}
	return out
}

func copyListItems(in []stream.ListItem) []events.ListItem {
	out := make([]events.ListItem, len(in))
	for i, it := range in {
		out[i] = events.ListItem{Status: it.Status, Text: it.Text}
	}
	return out
}

func copyResourceDetail(d stream.ResourceDetail) events.ResourceDetail {
	out := events.ResourceDetail{
		Kind: d.Kind, Name: d.Name, Namespace: d.Namespace,
		Status: d.Status, RecentLogs: d.RecentLogs, Labels: d.Labels,
	}
	for _, c := range d.Containers {
		out.Containers = append(out.Containers, events.ContainerInfo{
			Name: c.Name, Image: c.Image, Ready: c.Ready,
			RestartCount: c.RestartCount, State: c.State, Resources: c.Resources,
		})
	}
	for _, c := range d.Conditions {
		out.Conditions = append(out.Conditions, events.ConditionInfo{
			Type: c.Type, Status: c.Status, Reason: c.Reason, Message: c.Message,
		})
	}
	for _, ev := range d.Events {
		out.Events = append(out.Events, events.EventInfo{
			Type: ev.Type, Reason: ev.Reason, Message: ev.Message, Timestamp: ev.Timestamp,
		})
	}
	return out
}
