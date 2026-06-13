import { useMemo, useState } from 'react';
import type { DiagnosisEvent } from '../api/types';

interface DiagnosisTracePanelProps {
  events: DiagnosisEvent[];
  running: boolean;
  onBack: () => void;
}

export default function DiagnosisTracePanel({ events, running, onBack }: DiagnosisTracePanelProps) {
  const sorted = useMemo(
    () => [...events].sort((a, b) => a.seq_num - b.seq_num),
    [events],
  );
  const baseTime = sorted[0]?.created_at ?? 0;

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-3">
        <button
          type="button"
          onClick={onBack}
          className="text-xs px-3 py-1.5 rounded-sm border border-border text-text-secondary
                     hover:bg-hover hover:text-text transition-colors cursor-pointer bg-transparent
                     inline-flex items-center gap-1.5"
        >
          <span className="font-mono text-[10px]">←</span>
          返回报告
        </button>
        <span className="text-[10px] uppercase tracking-wider text-text-muted font-semibold">
          运行日志
        </span>
      </div>

      <p className="text-xs text-text-muted leading-relaxed">
        诊断管道每一步的执行记录。错误和降级的 AI 步骤会高亮显示，以便查看报告使用回退逻辑的原因。
      </p>

      {sorted.length === 0 ? (
        <div className="px-4 py-6 rounded-sm border border-border/60 bg-elevated/20 text-center text-sm text-text-muted">
          {running ? '等待管道事件…' : '无运行日志事件记录。'}
        </div>
      ) : (
        <div className="space-y-2">
          {sorted.map((ev) => (
            <TraceEventRow key={ev.seq_num} event={ev} baseTime={baseTime} />
          ))}
          {running && (
            <div className="flex items-center gap-2 px-3 py-2 text-xs text-accent font-mono">
              <span className="w-1.5 h-1.5 rounded-full bg-accent animate-pulse" />
              调查进行中…
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function TraceEventRow({ event, baseTime }: { event: DiagnosisEvent; baseTime: number }) {
  const [expanded, setExpanded] = useState(false);
  const meta = traceMeta(event);
  const payload = formatPayload(event);
  const hasPayload = Boolean(payload);
  const offset = formatOffset(event.created_at, baseTime);
  const detail = traceDetail(event);
  const subtitle = traceSubtitle(event);

  return (
    <div className={`rounded-sm border px-3 py-2.5 ${meta.borderClass} ${meta.bgClass}`}>
      <div className="flex items-start gap-2">
        <span
          className={`shrink-0 mt-0.5 text-[10px] font-mono uppercase tracking-wide px-1.5 py-0.5 rounded-sm ${meta.badgeClass}`}
        >
          {meta.label}
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex items-start justify-between gap-2">
            <p className={`text-sm leading-snug ${meta.textClass}`}>{traceTitle(event)}</p>
            <span className="text-[10px] text-text-muted font-mono shrink-0">
              #{event.seq_num}
              {offset ? ` · ${offset}` : ''}
            </span>
          </div>

          {subtitle && (
            <p className="text-xs text-text-secondary mt-1 leading-relaxed">{subtitle}</p>
          )}

          {detail && (
            <p
              className={`text-xs mt-1.5 leading-relaxed font-mono whitespace-pre-wrap break-words ${meta.detailClass}`}
            >
              {detail}
            </p>
          )}

          {event.elapsed_ms != null && event.elapsed_ms > 0 && (
            <p className="text-[10px] text-text-muted font-mono mt-1">
              elapsed {formatDurationMs(event.elapsed_ms)}
            </p>
          )}

          {hasPayload && (
            <button
              type="button"
              onClick={() => setExpanded((v) => !v)}
              className="text-[10px] text-text-muted hover:text-text mt-1.5 cursor-pointer bg-transparent border-0 p-0"
            >
              {expanded ? '隐藏详情' : '显示详情'}
            </button>
          )}

          {expanded && payload && (
            <pre className="text-[10px] text-text-muted font-mono whitespace-pre-wrap break-words mt-2 p-2 rounded-sm bg-bg/60 border border-border/40 max-h-48 overflow-y-auto">
              {payload}
            </pre>
          )}
        </div>
      </div>
    </div>
  );
}

function traceTitle(event: DiagnosisEvent): string {
  switch (event.event_type) {
    case 'phase':
      if (event.payload_kind === 'diagnosis_llm_step') {
        return event.summary || humanizeLLMStep(readStepKey(event.payload_json) || 'LLM step');
      }
      return event.summary || event.message?.replace(/^diagnosis\./, '') || 'phase';
    case 'tool_call':
      return `Running ${event.message || 'tool'}`;
    case 'tool_done':
      return `${event.message || 'tool'} completed`;
    case 'tool_fail':
      return `${event.message || 'tool'} failed`;
    case 'llm_step_degraded':
      return `AI step failed: ${humanizeLLMStep(event.message || 'unknown')}`;
    case 'agent_start':
      return event.message || 'Diagnosis started';
    case 'agent_done':
      return event.summary || 'Diagnosis finished';
    case 'stream_err':
      return 'Diagnosis failed';
    case 'stream_done':
      return 'Stream closed';
    default:
      return event.summary || event.message || event.event_type;
  }
}

function traceSubtitle(event: DiagnosisEvent): string | null {
  if (event.event_type === 'llm_step_degraded' && event.summary) {
    return `Fallback: ${event.summary}`;
  }
  if (event.event_type === 'phase' && event.message?.startsWith('diagnosis.')) {
    const stage = event.message.replace(/^diagnosis\./, '');
    if (event.summary && !event.summary.toLowerCase().includes(stage)) {
      return `Stage: ${stage}`;
    }
  }
  if (
    event.event_type === 'tool_done' &&
    event.summary &&
    event.summary !== event.message &&
    !event.summary.endsWith('completed')
  ) {
    return event.summary;
  }
  return null;
}

function traceDetail(event: DiagnosisEvent): string | null {
  if (event.event_type === 'agent_done') {
    return null;
  }
  if (event.event_type === 'llm_step_degraded') {
    return event.detail || null;
  }
  if (event.event_type === 'tool_fail' || event.event_type === 'stream_err') {
    return event.detail || event.summary || null;
  }
  if (event.detail && event.detail !== event.summary) {
    return event.detail;
  }
  return null;
}

function readStepKey(payloadJson?: string): string | null {
  if (!payloadJson) return null;
  try {
    const data = JSON.parse(payloadJson) as { step?: string };
    return data.step || null;
  } catch {
    return null;
  }
}

function humanizeLLMStep(step: string): string {
  const map: Record<string, string> = {
    llm_investigation_plan: 'investigation planning',
    llm_hypothesis_synthesis: 'hypothesis synthesis',
    llm_semantic_verification: 'semantic verification',
    llm_report_composition: 'report composition',
    llm_claim_audit: 'claim audit',
    llm_claim_audit_rejected: 'claim audit rejected',
  };
  return map[step] || step.replace(/^llm_/, '');
}

function traceMeta(event: DiagnosisEvent): {
  label: string;
  badgeClass: string;
  borderClass: string;
  bgClass: string;
  textClass: string;
  detailClass: string;
} {
  switch (event.event_type) {
    case 'stream_err':
    case 'tool_fail':
      return {
        label: 'error',
        badgeClass: 'bg-red-dim/30 text-red border border-red/20',
        borderClass: 'border-red/25',
        bgClass: 'bg-red-dim/8',
        textClass: 'text-red font-medium',
        detailClass: 'text-red/90',
      };
    case 'llm_step_degraded':
      return {
        label: 'degraded',
        badgeClass: 'bg-amber-dim/30 text-amber border border-amber/20',
        borderClass: 'border-amber/25',
        bgClass: 'bg-amber-dim/8',
        textClass: 'text-amber font-medium',
        detailClass: 'text-amber/90',
      };
    case 'tool_done':
    case 'agent_done':
      return {
        label: 'ok',
        badgeClass: 'bg-green-dim/30 text-green border border-green/20',
        borderClass: 'border-green/15',
        bgClass: 'bg-green-dim/5',
        textClass: 'text-text',
        detailClass: 'text-text-secondary',
      };
    case 'tool_call':
    case 'phase':
      return {
        label: event.payload_kind === 'diagnosis_llm_step' ? 'ai' : 'step',
        badgeClass: 'bg-accent-dim/20 text-accent border border-accent/15',
        borderClass: 'border-border/40',
        bgClass: 'bg-elevated/20',
        textClass: 'text-text',
        detailClass: 'text-text-secondary',
      };
    default:
      return {
        label: event.event_type.replace(/_/g, ' '),
        badgeClass: 'bg-elevated text-text-muted border border-border/40',
        borderClass: 'border-border/40',
        bgClass: 'bg-elevated/10',
        textClass: 'text-text-secondary',
        detailClass: 'text-text-muted',
      };
  }
}

function toEpochMs(value: number | undefined): number {
  if (!value) return 0;
  return value > 1_000_000_000_000 ? value : value * 1000;
}

function formatOffset(createdAt: number | undefined, baseTime: number): string {
  const deltaMs = toEpochMs(createdAt) - toEpochMs(baseTime);
  if (deltaMs <= 0) return '+0s';
  if (deltaMs < 1000) return `+${deltaMs}ms`;
  const secs = Math.round(deltaMs / 1000);
  if (secs < 60) return `+${secs}s`;
  const mins = Math.floor(secs / 60);
  const rem = secs % 60;
  return rem > 0 ? `+${mins}m${rem}s` : `+${mins}m`;
}

function formatDurationMs(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  const secs = Math.round(ms / 1000);
  if (secs < 60) return `${secs}s`;
  const mins = Math.floor(secs / 60);
  const rem = secs % 60;
  return rem > 0 ? `${mins}m ${rem}s` : `${mins}m`;
}

function formatPayload(event: DiagnosisEvent): string {
  const raw = event.payload_json;
  if (!raw) return '';
  try {
    const parsed = JSON.parse(raw) as unknown;
    if (event.payload_kind === 'markdown' && parsed && typeof parsed === 'object') {
      const markdown = (parsed as { markdown?: string }).markdown;
      if (markdown) {
        return markdown.length > 400 ? `${markdown.slice(0, 400)}…` : markdown;
      }
    }
    return JSON.stringify(parsed, null, 2);
  } catch {
    return raw;
  }
}
