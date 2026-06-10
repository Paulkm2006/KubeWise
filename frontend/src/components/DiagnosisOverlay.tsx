import { useState, useEffect } from 'react';
import { diagnosisSteps } from '../data/mock';
import type { StoredDiagnosis } from '../stores/diagnosisStore';
import type { DiagnosisEvent } from '../api/types';

interface DiagnosisOverlayProps {
  open: boolean;
  diagnosis: StoredDiagnosis | null;
  onClose: () => void;
  onActivity: (type: string, text: string, cluster?: string) => void;
}

export default function DiagnosisOverlay({ open, diagnosis, onClose, onActivity }: DiagnosisOverlayProps) {
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (diagnosis?.status === 'completed' || diagnosis?.status === 'failed') {
      const action = diagnosis.status === 'completed' ? 'done' : 'issue';
      onActivity(action, `${diagnosis.target.pod} diagnosis ${diagnosis.status}`, diagnosis.target.cluster);
    }
  }, [diagnosis?.status]);

  if (!open || !diagnosis) return null;

  const { events, status, target, result } = diagnosis;
  const isDone = status === 'completed' || status === 'failed';
  const latestPhase = findLatestPhase(events);
  const toolEvents = getToolEvents(events);
  const thoughtText = getLatestThought(events);
  const currentStep = computeStep(events);

  const handleCopy = async () => {
    const cmd = result?.fix_actions?.[0]?.command;
    if (!cmd) return;
    try {
      await navigator.clipboard.writeText(cmd);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch { /* fallback */ }
  };

  return (
    <>
      <div className="fixed inset-0 z-40 bg-black/40" onClick={onClose} />
      <div className="fixed top-0 right-0 bottom-0 w-[520px] z-50 bg-surface border-l border-border shadow-2xl
                      overflow-y-auto animate-slide-in">
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-border">
          <div>
            <h2 className="text-sm font-semibold text-text">
              {isDone ? '◆' : '◇'} {target.pod}
            </h2>
            <p className="text-xs text-text-muted mt-0.5 font-mono">
              {target.cluster} / {target.namespace}
            </p>
          </div>
          <button
            onClick={onClose}
            className="w-8 h-8 flex items-center justify-center rounded-sm border border-border
                       hover:bg-hover hover:text-text text-text-muted transition-colors cursor-pointer bg-transparent"
          >
            ✕
          </button>
        </div>

        <div className="p-6 space-y-5">
          {/* Progress */}
          {!isDone && (
            <div className="space-y-3">
              {diagnosisSteps.map((step, i) => {
                const isActive = currentStep === i + 1;
                const isStepDone = currentStep > i + 1;
                return (
                  <div
                    key={step.id}
                    className={`flex items-center gap-3 px-4 py-3 rounded-sm border transition-all duration-300 ${
                      isActive
                        ? 'border-accent/30 bg-accent-dim/10'
                        : isStepDone
                        ? 'border-green/20 bg-green-dim/10'
                        : 'border-border/50 bg-transparent'
                    }`}
                  >
                    <span className={`w-6 h-6 flex items-center justify-center rounded-full text-xs font-mono shrink-0
                      ${isActive ? 'bg-accent text-bg' : isStepDone ? 'bg-green text-bg' : 'bg-elevated text-text-muted border border-border'}`}
                    >
                      {isStepDone ? '✓' : isActive ? '◎' : step.id}
                    </span>
                    <div className="flex-1 min-w-0">
                      <p className={`text-sm ${isActive ? 'text-text font-medium' : isStepDone ? 'text-green' : 'text-text-muted'}`}>
                        {step.label}
                      </p>
                      <p className={`text-xs mt-0.5 font-mono ${isActive ? 'text-accent' : 'text-text-muted'}`}>
                        {isActive
                          ? (latestPhase || thoughtText?.slice(0, 80) || step.detail)
                          : isStepDone ? 'Complete' : 'Waiting...'}
                      </p>
                    </div>
                    {isActive && <span className="w-1.5 h-1.5 rounded-full bg-accent animate-ping" />}
                  </div>
                );
              })}
            </div>
          )}

          {/* Tool Execution Feed */}
          {!isDone && toolEvents.length > 0 && (
            <div>
              <h3 className="text-xs text-text-muted font-semibold tracking-wider uppercase mb-2">Tool Execution</h3>
              <div className="space-y-1.5">
                {toolEvents.slice(-5).map((ev, i) => (
                  <div key={i} className="flex items-center gap-2 px-3 py-2 rounded-sm bg-elevated border border-border/30 text-xs font-mono">
                    {renderToolEvent(ev)}
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Report */}
          {isDone && result && (
            <div className="space-y-5 animate-fade-in">
              <div className="flex items-center gap-1.5 text-xs text-text-muted font-mono">
                <span>⏱</span> Completed in {formatDuration(result.duration_ms || 0)}
              </div>

              <div className="px-5 py-4 rounded-sm bg-red-dim/15 border border-red/20">
                <p className="text-xs text-red font-semibold tracking-wider uppercase mb-1">Root Cause</p>
                <p className="text-base font-semibold text-text">{result.root_cause}</p>
                <p className="text-sm text-text-secondary mt-1">Confidence: {result.confidence || 'N/A'}</p>
              </div>

              {result.evidence && result.evidence.length > 0 && (
                <div>
                  <h3 className="text-xs text-text-muted font-semibold tracking-wider uppercase mb-3">Evidence Chain</h3>
                  <div className="space-y-2">
                    {result.evidence.map((e) => (
                      <div key={e.num} className="flex items-start gap-3 px-4 py-3 rounded-sm bg-elevated border border-border/30">
                        <span className="text-xs text-accent font-semibold font-mono shrink-0 w-5">{e.num}.</span>
                        <span className="text-sm text-text-secondary leading-relaxed">{e.text}</span>
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {result.fix_actions && result.fix_actions.length > 0 && (
                <div>
                  <h3 className="text-xs text-text-muted font-semibold tracking-wider uppercase mb-3">Fix Suggestion</h3>
                  <div className="px-4 py-3 rounded-sm bg-elevated border border-border">
                    <pre className="text-sm text-accent font-mono whitespace-pre-wrap leading-relaxed">
                      {result.fix_actions[0].command || result.fix_actions[0].description}
                    </pre>
                  </div>
                  {result.fix_actions[0].command && (
                    <div className="flex items-center gap-3 mt-3">
                      <button
                        onClick={handleCopy}
                        className="text-xs px-3 py-1.5 rounded-sm border border-accent/30 text-accent
                                   hover:bg-accent-dim/15 transition-colors cursor-pointer bg-transparent"
                      >
                        {copied ? '✓ Copied' : 'Copy Command'}
                      </button>
                    </div>
                  )}
                </div>
              )}

              {result.impact && (
                <div className="px-4 py-3 rounded-sm bg-elevated border border-border/50 text-sm text-text-muted leading-relaxed">
                  ⚠ Impact: {result.impact}
                </div>
              )}
            </div>
          )}

          {isDone && !result && (
            <div className="px-4 py-6 text-center text-text-muted text-sm">
              {status === 'failed' ? '✗ Diagnosis failed' : '✓ Diagnosis completed'}
            </div>
          )}
        </div>
      </div>
    </>
  );
}

// --- Helpers (render by event_type, no keyword guessing) ---

function computeStep(events: DiagnosisEvent[]): number {
  for (const ev of events) {
    if (ev.event_type !== 'phase' || !ev.message) continue;
    const msg = ev.message.toLowerCase();
    if (msg.includes('classifying') || msg.includes('collect')) return 1;
    if (msg.includes('troubleshooting') || msg.includes('analyz')) return 2;
    if (msg.includes('verif')) return 3;
    if (msg.includes('complete') || msg.includes('report')) return 4;
  }
  if (events.some(e => e.event_type === 'agent_done')) return 4;
  return events.length > 0 ? 1 : 0;
}

function findLatestPhase(events: DiagnosisEvent[]): string | null {
  for (let i = events.length - 1; i >= 0; i--) {
    if (events[i].event_type === 'phase' && events[i].message) {
      return events[i].message || null;
    }
  }
  return null;
}

function getToolEvents(events: DiagnosisEvent[]): DiagnosisEvent[] {
  return events.filter(e =>
    e.event_type === 'tool_call' || e.event_type === 'tool_done' || e.event_type === 'tool_fail'
  );
}

function getLatestThought(events: DiagnosisEvent[]): string | null {
  for (let i = events.length - 1; i >= 0; i--) {
    if (events[i].event_type === 'llm_text_delta') {
      return events[i].message || null;
    }
  }
  return null;
}

function renderToolEvent(ev: DiagnosisEvent): JSX.Element {
  switch (ev.event_type) {
    case 'tool_call':
      return <><span className="text-accent">▶</span> {ev.message}</>;
    case 'tool_done':
      return <><span className="text-green">✓</span> {ev.message} {ev.elapsed_ms ? `(${ev.elapsed_ms}ms)` : ''}</>;
    case 'tool_fail':
      return <><span className="text-red">✗</span> {ev.message}: {ev.detail}</>;
    default:
      return <><span>●</span> {ev.message}</>;
  }
}

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  const secs = Math.round(ms / 1000);
  if (secs < 60) return `${secs}s`;
  const mins = Math.floor(secs / 60);
  const rem = secs % 60;
  return `${mins}m ${rem}s`;
}