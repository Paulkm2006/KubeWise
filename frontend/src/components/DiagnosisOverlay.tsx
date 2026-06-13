import { useEffect, useState } from 'react';
import { DIAGNOSIS_STAGES } from '../api/types';
import type { StoredDiagnosis } from '../stores/diagnosisStore';
import type { DiagnosisEvent } from '../api/types';
import DiagnosisTracePanel from './DiagnosisTracePanel';

type DiagPhase = 'idle' | 'loading' | 'ready' | 'error';

interface DiagnosisOverlayProps {
  open: boolean;
  phase: DiagPhase;
  error: string | null;
  target: { cluster: string; namespace: string; pod: string };
  diagnosis: StoredDiagnosis | null;
  onClose: () => void;
  onRerun: () => void;
}

export default function DiagnosisOverlay({
  open,
  phase,
  error,
  target,
  diagnosis,
  onClose,
  onRerun,
}: DiagnosisOverlayProps) {
  const [copied, setCopied] = useState(false);
  const [rerunning, setRerunning] = useState(false);
  const [panelView, setPanelView] = useState<'report' | 'trace'>('report');

  useEffect(() => {
    if (!open) {
      setPanelView('report');
    }
  }, [open, diagnosis?.id]);

  if (!open) return null;

  const displayTarget = diagnosis?.target ?? {
    cluster: target.cluster,
    cluster_display: target.cluster,
    namespace: target.namespace,
    pod: target.pod,
  };

  const isLoading = phase === 'loading' || (phase === 'ready' && !diagnosis);
  const isError = phase === 'error';
  const status = diagnosis?.status ?? 'running';
  const isTerminal = status === 'completed' || status === 'failed' || status === 'cancelled';
  const result = diagnosis?.result;
  const derived = diagnosis?.derived;
  const stages = derived?.stages ?? [];
  const toolEvents = derived?.toolEvents ?? [];
  const traceEvents = diagnosis?.events ?? [];
  const diagnosedAt = diagnosis?.createdAt;
  const showTraceToggle = !isLoading && !isError && traceEvents.length > 0;

  const handleCopy = async (cmd: string) => {
    if (!cmd) return;
    try {
      await navigator.clipboard.writeText(cmd);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      /* fallback */
    }
  };

  const handleRerun = async () => {
    setRerunning(true);
    try {
      await onRerun();
    } finally {
      setRerunning(false);
    }
  };

  const primaryAction = result?.actions?.[0];

  return (
    <>
      <div className="fixed inset-0 z-40 bg-black/40" onClick={onClose} />
      <div
        className="fixed top-0 right-0 bottom-0 w-[520px] z-50 bg-surface border-l border-border shadow-2xl
                      overflow-y-auto animate-slide-in"
      >
        <div className="flex items-center justify-between px-6 py-4 border-b border-border bg-elevated/30">
          <div>
            <p className="text-[10px] uppercase tracking-[0.18em] text-text-muted font-semibold mb-1">
              诊断档案
            </p>
            <h2 className="text-sm font-semibold text-text">{displayTarget.pod}</h2>
            <p className="text-xs text-text-muted mt-0.5 font-mono">
              {displayTarget.cluster_display || displayTarget.cluster} / {displayTarget.namespace}
            </p>
          </div>
          <div className="flex items-center gap-2">
            {showTraceToggle && panelView === 'report' && diagnosis && (
              <button
                type="button"
                onClick={() => setPanelView('trace')}
                className="text-[10px] uppercase tracking-wider px-2.5 py-1.5 rounded-sm border border-border
                           text-text-muted hover:text-text hover:bg-hover transition-colors cursor-pointer bg-transparent"
              >
                运行日志
              </button>
            )}
            <button
              onClick={onClose}
              className="w-8 h-8 flex items-center justify-center rounded-sm border border-border
                         hover:bg-hover hover:text-text text-text-muted transition-colors cursor-pointer bg-transparent"
            >
              ✕
            </button>
          </div>
        </div>

        <div className="p-6 space-y-5">
          {panelView === 'trace' && diagnosis && (
            <DiagnosisTracePanel
              events={traceEvents}
              running={!isTerminal}
              onBack={() => setPanelView('report')}
            />
          )}

          {panelView === 'report' && (
            <>
          {diagnosedAt && !isLoading && (
            <div className="px-4 py-3 rounded-sm border border-border/60 bg-elevated/40">
              <p className="text-[10px] uppercase tracking-wider text-text-muted font-semibold">
                上次诊断
              </p>
              <p className="text-sm text-text mt-1">{formatAbsoluteTime(diagnosedAt)}</p>
              <p className="text-xs text-text-muted font-mono mt-0.5">
                {formatRelativeTime(diagnosedAt)}
              </p>
            </div>
          )}

          {isLoading && (
            <div className="px-5 py-8 rounded-sm border border-border bg-elevated/20 text-center space-y-4">
              <div className="inline-flex items-center gap-2 text-sm text-text-secondary">
                <span className="w-2 h-2 rounded-full bg-accent animate-pulse" />
                检查已有诊断
              </div>
              <p className="text-xs text-text-muted leading-relaxed max-w-sm mx-auto">
                正在恢复此 Pod 的最新诊断记录。如不存在，将开始新的诊断。
              </p>
            </div>
          )}

          {isError && (
            <div className="px-4 py-4 rounded-sm border border-red/30 bg-red-dim/10">
              <p className="text-xs text-red font-semibold uppercase tracking-wider mb-1">
                档案不可用
              </p>
              <p className="text-sm text-text-secondary">{error || '打开诊断失败。'}</p>
            </div>
          )}

          {!isLoading && !isError && diagnosis && !isTerminal && (
            <>
              <StageProgress stages={stages} liveCounts={derived} />

              {toolEvents.length > 0 && (
                <div>
                  <h3 className="text-xs text-text-muted font-semibold tracking-wider uppercase mb-2">
                    工具执行
                  </h3>
                  <div className="space-y-1.5">
                    {toolEvents.slice(-5).map((ev, i) => (
                      <div
                        key={i}
                        className="flex items-center gap-2 px-3 py-2 rounded-sm bg-elevated border border-border/30 text-xs font-mono"
                      >
                        {renderToolEvent(ev)}
                      </div>
                    ))}
                  </div>
                </div>
              )}

              <InvestigationRefreshPanel
                variant="in_progress"
                loading={rerunning}
                onAction={handleRerun}
              />

              {showTraceToggle && (
                <button
                  type="button"
                  onClick={() => setPanelView('trace')}
                  className="w-full text-xs px-3 py-2 rounded-sm border border-border/60 text-text-muted
                             hover:text-text hover:bg-hover transition-colors cursor-pointer bg-transparent"
                >
                  查看实时运行日志 →
                </button>
              )}
            </>
          )}

          {!isLoading && !isError && diagnosis && isTerminal && (
            <div className="space-y-5 animate-fade-in">
              {status === 'cancelled' && (
                <div className="px-4 py-3 rounded-sm border border-amber/30 bg-amber-dim/10">
                  <p className="text-xs font-semibold uppercase tracking-wider text-amber">
                    调查已停止
                  </p>
                  <p className="text-sm text-text-secondary mt-1 leading-relaxed">
                    此案例在报告完成前已关闭。
                  </p>
                </div>
              )}

              {status === 'failed' && !result && (
                <div className="px-4 py-6 text-center text-text-muted text-sm">
                  诊断在生成报告前失败。
                </div>
              )}

              {result && (
                <>
                  {result.enrichment?.status === 'degraded' && (
                    <div className="px-4 py-3 rounded-sm border border-amber/30 bg-amber-dim/10">
                      <p className="text-xs font-semibold uppercase tracking-wider text-amber">
                        降级分析
                      </p>
                      <p className="text-sm text-text-secondary mt-1 leading-relaxed">
                        {result.enrichment.message ||
                          '部分 AI 分析步骤暂时不可用，此报告使用基于已收集证据的确定性回退方案。'}
                      </p>
                      {result.enrichment.degraded_steps &&
                        result.enrichment.degraded_steps.length > 0 && (
                          <p className="text-xs text-text-muted mt-2 font-mono">
                            受影响: {result.enrichment.degraded_steps.join(', ')}
                          </p>
                        )}
                    </div>
                  )}

                  {result.duration_ms != null && result.duration_ms > 0 && (
                    <div className="flex items-center gap-1.5 text-xs text-text-muted font-mono">
                      <span>⏱</span> 耗时 {formatDuration(result.duration_ms)}
                    </div>
                  )}

                  <div className="px-5 py-4 rounded-sm bg-red-dim/15 border border-red/20">
                    <p className="text-xs text-red font-semibold tracking-wider uppercase mb-1">
                      根因
                    </p>
                    <p className="text-base font-semibold text-text">
                      {result.root_cause.title || result.root_cause.summary}
                    </p>
                    <p className="text-sm text-text-secondary mt-1 leading-relaxed">
                      {result.root_cause.summary}
                    </p>
                    <div className="flex flex-wrap gap-2 mt-2 text-xs">
                      {result.verdict && (
                        <span className="px-2 py-0.5 rounded-sm bg-elevated border border-border font-mono uppercase">
                          {result.verdict}
                        </span>
                      )}
                      {result.root_cause.confidence_label && (
                        <span className="text-text-muted">
                          置信度: {result.root_cause.confidence_label}
                          {result.root_cause.confidence_score != null &&
                            ` (${Math.round(result.root_cause.confidence_score * 100)}%)`}
                        </span>
                      )}
                      {result.root_cause.category && (
                        <span className="text-text-muted font-mono">{result.root_cause.category}</span>
                      )}
                    </div>
                  </div>

                  {result.evidence && result.evidence.length > 0 && (
                    <div>
                      <h3 className="text-xs text-text-muted font-semibold tracking-wider uppercase mb-3">
                        证据链
                      </h3>
                      <div className="space-y-2">
                        {result.evidence.map((e) => (
                          <div
                            key={e.id}
                            className="flex items-start gap-3 px-4 py-3 rounded-sm bg-elevated border border-border/30"
                          >
                            <span className="text-xs text-accent font-semibold font-mono shrink-0">
                              {e.id}
                            </span>
                            <div className="min-w-0">
                              <p className="text-sm text-text-secondary leading-relaxed">{e.summary}</p>
                              {(e.source || e.strength) && (
                                <p className="text-[11px] text-text-muted font-mono mt-1">
                                  {[e.source, e.strength, e.signal].filter(Boolean).join(' · ')}
                                </p>
                              )}
                            </div>
                          </div>
                        ))}
                      </div>
                    </div>
                  )}

                  {result.hypotheses && result.hypotheses.length > 0 && (
                    <div>
                      <h3 className="text-xs text-text-muted font-semibold tracking-wider uppercase mb-3">
                        已验证假设
                      </h3>
                      <div className="space-y-2">
                        {result.hypotheses.map((h) => (
                          <div
                            key={h.id}
                            className="px-4 py-3 rounded-sm bg-elevated border border-border/30"
                          >
                            <div className="flex items-center justify-between gap-2">
                              <p className="text-sm text-text font-medium">{h.title}</p>
                              <span className="text-[10px] uppercase font-mono text-text-muted">
                                {h.status}
                              </span>
                            </div>
                            {h.rationale && (
                              <p className="text-xs text-text-secondary mt-1 leading-relaxed">
                                {h.rationale}
                              </p>
                            )}
                          </div>
                        ))}
                      </div>
                    </div>
                  )}

                  {result.actions && result.actions.length > 0 && (
                    <div>
                      <h3 className="text-xs text-text-muted font-semibold tracking-wider uppercase mb-3">
                        建议操作
                      </h3>
                      <div className="space-y-2">
                        {result.actions.map((action, i) => (
                          <div
                            key={i}
                            className="px-4 py-3 rounded-sm bg-elevated border border-border"
                          >
                            <p className="text-sm text-text-secondary">
                              {action.priority && (
                                <span className="text-accent font-mono text-xs mr-2">
                                  {action.priority}
                                </span>
                              )}
                              {action.description}
                            </p>
                            {action.command && (
                              <pre className="text-xs text-accent font-mono whitespace-pre-wrap leading-relaxed mt-2">
                                {action.command}
                              </pre>
                            )}
                          </div>
                        ))}
                      </div>
                      {primaryAction?.command && (
                        <div className="flex items-center gap-3 mt-3">
                          <button
                            onClick={() => handleCopy(primaryAction.command!)}
                            className="text-xs px-3 py-1.5 rounded-sm border border-accent/30 text-accent
                                       hover:bg-accent-dim/15 transition-colors cursor-pointer bg-transparent"
                          >
                            {copied ? '✓ 已复制' : '复制命令'}
                          </button>
                        </div>
                      )}
                    </div>
                  )}

                  {result.impact?.description && (
                    <div className="px-4 py-3 rounded-sm bg-elevated border border-border/50 text-sm text-text-muted leading-relaxed">
                      影响 ({result.impact.severity || '未知'}): {result.impact.description}
                    </div>
                  )}

                  {result.limitations && result.limitations.length > 0 && (
                    <div>
                      <h3 className="text-xs text-text-muted font-semibold tracking-wider uppercase mb-2">
                        局限
                      </h3>
                      <ul className="space-y-1.5 text-sm text-text-muted">
                        {result.limitations.map((lim, i) => (
                          <li key={i} className="flex gap-2">
                            <span className="text-amber">•</span>
                            <span>{lim}</span>
                          </li>
                        ))}
                      </ul>
                    </div>
                  )}
                </>
              )}

              <InvestigationRefreshPanel
                variant={status === 'cancelled' ? 'cancelled' : status === 'failed' ? 'failed' : 'archived'}
                loading={rerunning}
                onAction={handleRerun}
              />

              {showTraceToggle && (
                <button
                  type="button"
                  onClick={() => setPanelView('trace')}
                  className="w-full text-xs px-3 py-2 rounded-sm border border-border/60 text-text-muted
                             hover:text-text hover:bg-hover transition-colors cursor-pointer bg-transparent"
                >
                  查看诊断执行过程 →
                </button>
              )}
            </div>
          )}
            </>
          )}
        </div>
      </div>
    </>
  );
}

function StageProgress({
  stages,
  liveCounts,
}: {
  stages: StoredDiagnosis['derived']['stages'];
  liveCounts?: StoredDiagnosis['derived'];
}) {
  return (
    <div className="space-y-3">
      {DIAGNOSIS_STAGES.map((meta) => {
        const state = stages.find((s) => s.id === meta.id);
        const isActive = state?.status === 'running';
        const isStepDone = state?.status === 'completed';
        const liveHint =
          meta.id === 'analyze' && liveCounts?.liveEvidenceCount
            ? `${liveCounts.liveEvidenceCount} signals`
            : meta.id === 'verify' && liveCounts?.liveHypothesisCount
            ? `${liveCounts.liveHypothesisCount} hypotheses`
            : null;

        return (
          <div
            key={meta.id}
            className={`flex items-center gap-3 px-4 py-3 rounded-sm border transition-all duration-300 ${
              isActive
                ? 'border-accent/30 bg-accent-dim/10'
                : isStepDone
                ? 'border-green/20 bg-green-dim/10'
                : 'border-border/50 bg-transparent'
            }`}
          >
            <span
              className={`w-6 h-6 flex items-center justify-center rounded-full text-xs font-mono shrink-0
              ${
                isActive
                  ? 'bg-accent text-bg'
                  : isStepDone
                  ? 'bg-green text-bg'
                  : 'bg-elevated text-text-muted border border-border'
              }`}
            >
              {isStepDone ? '✓' : isActive ? '◎' : meta.id.slice(0, 1).toUpperCase()}
            </span>
            <div className="flex-1 min-w-0">
              <p
                className={`text-sm ${
                  isActive
                    ? 'text-text font-medium'
                    : isStepDone
                    ? 'text-green'
                    : 'text-text-muted'
                }`}
              >
                {meta.label}
              </p>
              <p
                className={`text-xs mt-0.5 font-mono ${
                  isActive ? 'text-accent' : 'text-text-muted'
                }`}
              >
                {isActive || isStepDone
                  ? state?.summary || liveHint || meta.detail
                  : 'Waiting...'}
              </p>
            </div>
            {isActive && <span className="w-1.5 h-1.5 rounded-full bg-accent animate-ping" />}
          </div>
        );
      })}
    </div>
  );
}

type RefreshVariant = 'archived' | 'failed' | 'cancelled' | 'in_progress';

function InvestigationRefreshPanel({
  variant,
  loading,
  onAction,
}: {
  variant: RefreshVariant;
  loading: boolean;
  onAction: () => void;
}) {
  const copy = refreshCopy[variant];

  return (
    <div className="rounded-sm border border-border/60 bg-elevated/25 overflow-hidden">
      <div className="px-4 py-3 border-b border-border/30">
        <p className="text-xs font-medium text-text-secondary">{copy.title}</p>
        <p className="text-[11px] text-text-muted mt-1.5 leading-relaxed">{copy.body}</p>
      </div>
      <div className="px-4 py-3 flex items-center justify-between gap-4">
        <span className="text-[10px] uppercase tracking-[0.14em] text-text-muted font-semibold shrink-0">
          {copy.label}
        </span>
        <button
          type="button"
          onClick={onAction}
          disabled={loading}
          className="text-xs font-medium px-3.5 py-2 rounded-sm border border-accent/35 text-accent
                     hover:bg-accent-dim/15 hover:border-accent/50 transition-colors cursor-pointer
                     bg-transparent disabled:opacity-40 disabled:cursor-not-allowed
                     inline-flex items-center gap-2 shrink-0"
        >
          {loading ? (
            <>
              <span className="w-1.5 h-1.5 rounded-full bg-accent animate-pulse" />
              调查中…
            </>
          ) : (
            <>
              <span className="text-text-muted font-mono text-[10px]">↻</span>
              {copy.action}
            </>
          )}
        </button>
      </div>
    </div>
  );
}

const refreshCopy: Record<
  RefreshVariant,
  { title: string; body: string; label: string; action: string }
> = {
  archived: {
    title: '案例可能过时',
    body: 'Pod 状态可能已发生变化。新的调查将收集最新信号并替换当前发现。',
    label: '刷新',
    action: '运行新诊断',
  },
  failed: {
    title: '调查未完成',
    body: '上次运行未得出可靠结论。开始新的诊断以从当前集群状态重建案例。',
    label: '重试',
    action: '运行新诊断',
  },
  cancelled: {
    title: '无最终报告',
    body: '上次调查已提前停止。打开新案例以从实时数据继续排查。',
    label: '继续',
    action: '运行新诊断',
  },
  in_progress: {
    title: '需要重新开始？',
    body: '重新开始将停止当前调查并为此 Pod 打开新案例。',
    label: '重启',
    action: '新调查',
  },
};

function formatAbsoluteTime(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

function formatRelativeTime(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  const diffMs = Date.now() - date.getTime();
  const secs = Math.max(0, Math.round(diffMs / 1000));
  if (secs < 60) return `${secs}秒前`;
  const mins = Math.floor(secs / 60);
  if (mins < 60) return `${mins}分钟前`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}小时前`;
  const days = Math.floor(hours / 24);
  return `${days}天前`;
}

function renderToolEvent(ev: DiagnosisEvent): JSX.Element {
  const label = ev.summary || ev.message;
  switch (ev.event_type) {
    case 'tool_call':
      return (
        <>
          <span className="text-accent">▶</span> {label}
        </>
      );
    case 'tool_done':
      return (
        <>
          <span className="text-green">✓</span> {label}{' '}
          {ev.elapsed_ms ? `(${ev.elapsed_ms}ms)` : ''}
        </>
      );
    case 'tool_fail':
      return (
        <>
          <span className="text-red">✗</span> {label}: {ev.detail}
        </>
      );
    default:
      return (
        <>
          <span>●</span> {label}
        </>
      );
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
