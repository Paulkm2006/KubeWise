import type {
  ChatInteractionKind,
  ChatProgressCard,
  ChatProgressPhase,
  ChatSSEEvent,
  ChatToolLine,
} from '../api/types';

export function createProgressCard(queryId: string, agentName: string): ChatProgressCard {
  return {
    queryId,
    agentName,
    phases: [{ id: '0', label: agentName, tools: [], reasoningText: '', done: false, expanded: true }],
    done: false,
    failed: false,
    finalReport: '',
    detailsExpanded: false,
  };
}

function lastPhase(card: ChatProgressCard): ChatProgressPhase {
  return card.phases[card.phases.length - 1];
}

function closeLastPhase(card: ChatProgressCard) {
  const phase = lastPhase(card);
  if (!phase.done) phase.done = true;
}

function addPhase(card: ChatProgressCard, label: string) {
  closeLastPhase(card);
  card.phases.push({
    id: String(card.phases.length),
    label,
    tools: [],
    reasoningText: '',
    done: false,
    expanded: card.phases.length === 0,
  });
}

function findTool(phase: ChatProgressPhase, name: string, step: number): ChatToolLine | undefined {
  return phase.tools.find((t) => t.name === name && t.step === step);
}

export function foldChatEvent(card: ChatProgressCard, ev: ChatSSEEvent): ChatProgressCard {
  const next = structuredClone(card);

  switch (ev.type) {
    case 'agent_start':
      next.agentName = ev.agent_name || next.agentName;
      if (next.phases.length === 0) {
        next.phases = [{ id: '0', label: next.agentName, tools: [], reasoningText: '', done: false, expanded: true }];
      }
      break;

    case 'phase':
      if (ev.phase) addPhase(next, ev.phase);
      break;

    case 'supervisor':
      if (ev.decision || ev.detail) {
        addPhase(next, `supervisor: ${ev.decision || ''} — ${ev.detail || ''}`.trim());
      }
      break;

    case 'llm_text_delta':
      if (ev.delta) lastPhase(next).reasoningText += ev.delta;
      break;

    case 'tool_call':
      if (ev.tool_name) {
        lastPhase(next).tools.push({
          name: ev.tool_name,
          step: ev.step ?? 0,
          done: false,
          failed: false,
        });
      }
      break;

    case 'tool_done':
      if (ev.tool_name) {
        const tool = findTool(lastPhase(next), ev.tool_name, ev.step ?? 0);
        if (tool) {
          tool.done = true;
          tool.elapsedMs = ev.elapsed ? Math.round(ev.elapsed / 1_000_000) : undefined;
        }
      }
      break;

    case 'tool_fail':
      if (ev.tool_name) {
        const tool = findTool(lastPhase(next), ev.tool_name, ev.step ?? 0);
        if (tool) {
          tool.done = true;
          tool.failed = true;
          tool.elapsedMs = ev.elapsed ? Math.round(ev.elapsed / 1_000_000) : undefined;
        }
      }
      break;

    case 'agent_done':
      closeLastPhase(next);
      next.done = true;
      if (ev.result) next.finalReport = ev.result;
      if (ev.duration != null) next.durationMs = Math.round(ev.duration / 1_000_000);
      next.inTokens = ev.in_tokens;
      next.outTokens = ev.out_tokens;
      break;

    case 'interaction_request':
      if (ev.interaction_id && ev.kind) {
        next.awaitingInteraction = {
          interactionId: ev.interaction_id,
          kind: ev.kind as ChatInteractionKind,
          payload: ev.payload ?? {},
          totalSteps: ev.total_steps,
          resolved: false,
        };
      }
      break;

    case 'stream_err':
      next.failed = true;
      next.errorMsg = ev.error || 'stream error';
      next.done = true;
      break;

    case 'stream_done':
      next.done = true;
      if (!next.finalReport && next.phases.some((p) => p.reasoningText)) {
        next.finalReport = lastPhase(next).reasoningText;
      }
      break;
  }

  return next;
}

export function togglePhaseExpanded(card: ChatProgressCard, phaseId: string): ChatProgressCard {
  return {
    ...card,
    phases: card.phases.map((p) => (p.id === phaseId ? { ...p, expanded: !p.expanded } : p)),
  };
}

export function toggleDetailsExpanded(card: ChatProgressCard): ChatProgressCard {
  return { ...card, detailsExpanded: !card.detailsExpanded };
}
