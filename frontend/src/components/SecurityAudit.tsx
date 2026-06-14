import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { api } from '../api/client';
import type { AuditFinding } from '../api/types';
import { AuditStore, type StoredAudit } from '../stores/auditStore';

type PagePhase = 'loading' | 'empty' | 'running' | 'completed' | 'failed' | 'error';

interface SecurityAuditProps {
  activeCluster: string;
  store: AuditStore;
  onActivity: (type: string, text: string, cluster?: string, kind?: string, detail?: string) => void;
  onStoreUpdate: () => void;
  refreshKey?: number;
}

const sevStyle: Record<string, string> = {
  critical: 'text-red bg-red-dim',
  high: 'text-red bg-red-dim',
  medium: 'text-amber bg-amber-dim',
  low: 'text-accent bg-accent-dim',
};

const FILTER_KEYS = ['all', 'critical', 'high', 'medium', 'low'] as const;
const FILTER_SEVERITY: Record<string, string> = {
  all: '',
  critical: 'CRITICAL',
  high: 'HIGH',
  medium: 'MEDIUM',
  low: 'LOW',
};

function formatAge(iso?: string): string {
  if (!iso) return '';
  const ms = Date.now() - new Date(iso).getTime();
  const mins = Math.floor(ms / 60000);
  if (mins < 1) return '刚刚';
  if (mins < 60) return `${mins}分钟前`;
  const hours = Math.floor(mins / 60);
  if (hours < 48) return `${hours}小时前`;
  return `${Math.floor(hours / 24)}天前`;
}

function formatDuration(ms?: number): string {
  if (!ms) return '—';
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

function formatTimestamp(iso?: string): string {
  if (!iso) return '—';
  return new Date(iso).toISOString().slice(0, 16).replace('T', ' ') + ' UTC';
}

function isStale(iso?: string): 'fresh' | 'aging' | 'stale' {
  if (!iso) return 'fresh';
  const hours = (Date.now() - new Date(iso).getTime()) / 3600000;
  if (hours > 168) return 'stale';
  if (hours > 24) return 'aging';
  return 'fresh';
}

export default function SecurityAudit({
  activeCluster,
  store,
  onActivity,
  onStoreUpdate,
  refreshKey,
}: SecurityAuditProps) {
  const { t } = useTranslation();
  const [pagePhase, setPagePhase] = useState<PagePhase>('loading');
  const [error, setError] = useState<string | null>(null);
  const [stored, setStored] = useState<StoredAudit | null>(null);
  const [filter, setFilter] = useState('all');
  const [elapsedSec, setElapsedSec] = useState(0);
  const [actionPending, setActionPending] = useState(false);
  const [logOpen, setLogOpen] = useState(false);
  const runStartedAt = useRef<number | null>(null);

  const activeClusterRef = useRef(activeCluster);
  activeClusterRef.current = activeCluster;

  const storeCallbacks = useCallback(
    () => ({
      onUpdate: () => {
        onStoreUpdate();
        const current = store.getForCluster(activeClusterRef.current);
        if (current) {
          setStored({ ...current });
          if (current.status === 'running') setPagePhase('running');
          else if (current.status === 'completed') setPagePhase('completed');
          else if (current.status === 'failed') setPagePhase('failed');
        }
      },
      onTerminal: (s: StoredAudit) => {
        onActivity(
          s.status === 'failed' ? 'issue' : 'done',
          t('activity.auditCompleted', { status: s.status, count: s.result?.summary.total ?? 0 }),
          s.target.cluster,
          'audit',
        );
      },
    }),
    [onActivity, onStoreUpdate, store],
  );

  const loadCluster = useCallback(async () => {
    if (!activeCluster) return;
    setPagePhase('loading');
    setError(null);

    const running = store.findRunning(activeCluster);
    if (running) {
      setStored(running);
      setPagePhase('running');
      return;
    }

    const cached = store.getForCluster(activeCluster);
    if (cached?.status === 'running') {
      setStored(cached);
      setPagePhase('running');
      return;
    }

    try {
      const status = await api.audits.latest(activeCluster);
      const restored = store.restoreFromStatus(status, storeCallbacks());
      setStored(restored);
      if (restored.status === 'running') setPagePhase('running');
      else if (restored.status === 'completed') setPagePhase('completed');
      else if (restored.status === 'failed') setPagePhase('failed');
      else setPagePhase('empty');
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Unknown error';
      if (msg.includes('404')) {
        setStored(null);
        setPagePhase('empty');
        return;
      }
      setError(msg);
      setPagePhase('error');
    }
  }, [activeCluster, store, storeCallbacks]);

  useEffect(() => {
    loadCluster();
  }, [loadCluster, refreshKey]);

  useEffect(() => {
    if (pagePhase !== 'running') {
      runStartedAt.current = null;
      setElapsedSec(0);
      return;
    }
    if (!runStartedAt.current) {
      runStartedAt.current = Date.now();
    }
    const t = setInterval(() => {
      if (runStartedAt.current) {
        setElapsedSec(Math.floor((Date.now() - runStartedAt.current) / 1000));
      }
    }, 1000);
    return () => clearInterval(t);
  }, [pagePhase, stored?.id]);

  const liveStored = stored?.id ? store.get(stored.id) ?? stored : stored;
  const derived = liveStored?.derived;
  const findings: AuditFinding[] =
    pagePhase === 'completed'
      ? liveStored?.result?.findings ?? derived?.liveFindings ?? []
      : derived?.liveFindings ?? [];

  const summary = liveStored?.result?.summary ?? {
    total: findings.length,
    critical: findings.filter((f) => f.severity === 'critical').length,
    high: findings.filter((f) => f.severity === 'high').length,
    medium: findings.filter((f) => f.severity === 'medium').length,
    low: findings.filter((f) => f.severity === 'low').length,
  };

  const filtered = useMemo(() => {
    if (filter === 'all') return findings;
    const severity = FILTER_SEVERITY[filter];
    if (!severity) return findings;
    return findings.filter((f) => f.severity.toUpperCase() === severity);
  }, [findings, filter]);

  const handleStart = async () => {
    if (!activeCluster || actionPending) return;
    setActionPending(true);
    try {
      onActivity('pending', t('activity.auditing', { cluster: activeCluster }), activeCluster, 'audit');
      const res = await api.audits.create(activeCluster);
      runStartedAt.current = Date.now();
      const s = store.add(
        res.audit_id,
        { cluster: activeCluster, cluster_display: activeCluster },
        storeCallbacks(),
      );
      setStored(s);
      setPagePhase('running');
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Failed to start audit';
      setError(msg);
      setPagePhase('error');
      onActivity('issue', msg, activeCluster);
    } finally {
      setActionPending(false);
    }
  };

  const handleReaudit = async () => {
    if (!activeCluster || actionPending) return;
    setActionPending(true);
    try {
      if (liveStored?.status === 'running') {
        await api.audits.cancel(liveStored.id);
        store.closeSSE(liveStored.id);
      }
      onActivity('pending', t('activity.reAuditing', { cluster: activeCluster }), activeCluster, 'audit');
      const res = await api.audits.create(activeCluster);
      runStartedAt.current = Date.now();
      const s = store.add(
        res.audit_id,
        { cluster: activeCluster, cluster_display: activeCluster },
        storeCallbacks(),
      );
      setStored(s);
      setPagePhase('running');
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Re-audit failed';
      setError(msg);
      setPagePhase('error');
      onActivity('issue', msg, activeCluster);
    } finally {
      setActionPending(false);
    }
  };

  const handleCancel = async () => {
    if (!liveStored || actionPending) return;
    setActionPending(true);
    try {
      await api.audits.cancel(liveStored.id);
      store.closeSSE(liveStored.id);
      await loadCluster();
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Cancel failed';
      onActivity('issue', msg, activeCluster);
    } finally {
      setActionPending(false);
    }
  };

  const handleExport = () => {
    const md = liveStored?.result?.markdown;
    if (!md) return;
    const date = new Date().toISOString().slice(0, 10);
    const blob = new Blob([md], { type: 'text/markdown;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `kubewise-audit-${activeCluster}-${date}.md`;
    a.click();
    URL.revokeObjectURL(url);
    onActivity('done', t('activity.auditExported'), activeCluster, 'audit');
  };

  const progressPct =
    derived && derived.totalPhases > 0
      ? Math.round((derived.completedCount / derived.totalPhases) * 100)
      : 0;

  const freshness = isStale(liveStored?.createdAt);
  const canExport = pagePhase === 'completed' && Boolean(liveStored?.result?.markdown);

  return (
    <div className="h-full overflow-y-auto px-8 py-6">
      <div className="flex items-start justify-between mb-6 gap-4">
        <div>
          <h1 className="text-sm font-semibold text-text tracking-wide">{t('audit.title')}</h1>
          <p className="text-sm text-text-muted mt-1">
            {activeCluster || t('audit.noCluster')}
            {pagePhase === 'completed' && liveStored?.createdAt && (
              <span className="ml-2">
                · {t('audit.lastAudited')} {formatAge(liveStored.createdAt)}
                {freshness === 'aging' && (
                  <span className="text-amber ml-1">· {t('audit.outdated')}</span>
                )}
                {freshness === 'stale' && (
                  <span className="text-red ml-1">· {t('audit.stale')}</span>
                )}
              </span>
            )}
          </p>
        </div>

        <div className="flex items-center gap-2 shrink-0">
          {pagePhase === 'running' && (
            <button
              onClick={handleCancel}
              disabled={actionPending}
              className="text-sm text-text-muted px-4 py-1.5 border border-border rounded-sm
                         hover:border-red/40 hover:text-red transition-colors cursor-pointer bg-transparent"
            >
              {t('audit.cancel')}
            </button>
          )}
          {(pagePhase === 'completed' || pagePhase === 'failed') && (
            <button
              onClick={handleReaudit}
              disabled={actionPending}
              className="text-sm text-text-secondary px-4 py-1.5 border border-border rounded-sm
                         hover:border-accent/30 hover:text-text transition-colors cursor-pointer bg-transparent"
            >
              {t('audit.reaudit')}
            </button>
          )}
          <button
            onClick={handleExport}
            disabled={!canExport}
            title={canExport ? 'Download Markdown report' : 'Available after audit completes'}
            className="text-sm text-accent px-4 py-1.5 border border-accent/30 rounded-sm
                       hover:bg-accent-dim/15 transition-colors cursor-pointer bg-transparent font-medium
                       disabled:opacity-40 disabled:cursor-not-allowed"
          >
            {t('audit.export')}
          </button>
        </div>
      </div>

      {pagePhase === 'loading' && (
        <div className="flex items-center justify-center py-24 text-sm text-text-muted">
          {t('audit.loading')}
        </div>
      )}

      {pagePhase === 'error' && (
        <div className="text-center py-24">
          <p className="text-sm text-red mb-4">{error}</p>
          <button
            onClick={loadCluster}
            className="text-sm px-4 py-2 border border-border rounded-sm hover:bg-hover cursor-pointer bg-transparent"
          >
            {t('audit.retry')}
          </button>
        </div>
      )}

      {pagePhase === 'empty' && (
        <div className="flex flex-col items-center justify-center py-20 text-center max-w-md mx-auto">
          <span className="text-3xl text-text-muted mb-4">◇</span>
          <h2 className="text-base font-semibold text-text mb-2">{t('audit.emptyTitle')}</h2>
          <p className="text-sm text-text-muted leading-relaxed mb-6">
            {t('audit.emptyDesc')} <span className="text-text font-medium">{activeCluster}</span>。
          </p>
          <button
            onClick={handleStart}
            disabled={!activeCluster || actionPending}
            className="text-sm font-medium text-bg bg-accent px-6 py-2.5 rounded-sm
                       hover:brightness-110 transition-all cursor-pointer disabled:opacity-50"
          >
            {t('audit.runAudit')}
          </button>
          <p className="text-xs text-text-muted mt-6">
            {t('audit.scope')}
          </p>
        </div>
      )}

      {pagePhase === 'failed' && (
        <div className="mb-6 px-4 py-3 rounded-sm border border-red/30 bg-red-dim/10 text-sm text-red">
          {t('audit.failed')}{liveStored?.errorMessage ? `: ${liveStored.errorMessage}` : ''}
        </div>
      )}

      {pagePhase === 'running' && derived && (
        <div className="mb-6 border border-border rounded-sm p-5 bg-elevated/20">
          <div className="flex items-center justify-between mb-3">
            <span className="text-sm font-medium text-text">
              {t('audit.auditing')} {activeCluster}
            </span>
            <span className="text-sm font-mono text-text-muted">
              {Math.floor(elapsedSec / 60)}:{(elapsedSec % 60).toString().padStart(2, '0')}
            </span>
          </div>
          <div className="h-1.5 bg-border rounded-full overflow-hidden mb-4">
            <div
              className="h-full bg-accent transition-all duration-500 ease-out"
              style={{ width: `${Math.max(progressPct, 8)}%` }}
            />
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 mb-4">
            {derived.phases.map((phase) => (
              <div
                key={phase.id}
                className={`flex items-center gap-2 text-sm px-3 py-2 rounded-sm border ${
                  phase.status === 'running'
                    ? 'border-accent/40 bg-accent-dim/10'
                    : phase.status === 'completed'
                      ? 'border-border/60 bg-surface/50'
                      : 'border-border/30 text-text-muted'
                }`}
              >
                <PhaseIcon status={phase.status} />
                <span className="flex-1 font-medium">{phase.label}</span>
                {phase.status === 'completed' && phase.count !== undefined && (
                  <span className="text-xs font-mono text-text-muted">{t('audit.findings', { count: phase.count })}</span>
                )}
                {phase.status === 'running' && (
                  <span className="text-xs text-accent animate-pulse">{t('audit.scanning')}</span>
                )}
              </div>
            ))}
          </div>

          {findings.length > 0 && (
            <div>
              <p className="text-xs uppercase tracking-wider text-text-muted font-semibold mb-2">
                {t('audit.liveFindings', { count: findings.length })}
              </p>
              <div className="space-y-1.5 max-h-48 overflow-y-auto">
                {findings.slice(-8).map((row, i) => (
                  <LiveFindingRow key={`${row.resource}-${i}`} row={row} />
                ))}
              </div>
            </div>
          )}

          {liveStored && liveStored.events.length > 0 && (
            <button
              onClick={() => setLogOpen((v) => !v)}
              className="mt-4 text-xs text-text-muted hover:text-text cursor-pointer bg-transparent border-0"
            >
              {logOpen ? '▾' : '▸'} {t('audit.runLog', { count: liveStored.events.length })}
            </button>
          )}
          {logOpen && liveStored && (
            <div className="mt-2 space-y-1 max-h-32 overflow-y-auto text-xs font-mono text-text-muted">
              {liveStored.events.map((ev) => (
                <div key={ev.seq_num}>
                  [{ev.event_type}] {ev.summary || ev.message || ev.detail}
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {(pagePhase === 'running' || pagePhase === 'completed' || pagePhase === 'failed') &&
        findings.length > 0 && (
          <>
            <div className="grid grid-cols-5 gap-4 mb-6">
              {[
                { label: t('audit.total'), value: summary.total, color: 'text-text', key: 'all' },
                { label: t('audit.critical'), value: summary.critical, color: 'text-red', key: 'critical' },
                { label: t('audit.high'), value: summary.high, color: 'text-red', key: 'high' },
                { label: t('audit.medium'), value: summary.medium, color: 'text-amber', key: 'medium' },
                { label: t('audit.low'), value: summary.low, color: 'text-accent', key: 'low' },
              ].map((c) => (
                <button
                  key={c.label}
                  onClick={() => setFilter(c.key)}
                  className={`border rounded-sm p-5 text-center transition-all duration-150 cursor-pointer
                    ${filter === c.key
                      ? 'border-accent/40 bg-accent-dim/10'
                      : 'border-border bg-surface hover:bg-elevated'
                    }`}
                >
                  <p className={`text-2xl font-semibold font-mono ${c.color}`}>{c.value}</p>
                  <p className="text-sm text-text-muted mt-1">{c.label}</p>
                </button>
              ))}
            </div>

            <div className="flex gap-2 mb-5">
              {FILTER_KEYS.map((f) => (
                <button
                  key={f}
                  onClick={() => setFilter(f)}
                  className={`text-sm px-4 py-1.5 rounded-sm border transition-all duration-150 cursor-pointer font-medium
                    ${filter === f
                      ? 'border-accent/40 text-accent bg-accent-dim/15'
                      : 'border-border text-text-muted hover:border-accent/30 hover:text-text bg-transparent'
                    }`}
                >
                  {t(`audit.${f}`)}
                </button>
              ))}
            </div>

            <FindingsTable findings={filtered} dimmed={pagePhase === 'running'} />
          </>
        )}

      {pagePhase === 'completed' && findings.length === 0 && (
        <div className="text-center py-16 border border-border rounded-sm">
          <p className="text-lg font-semibold text-green mb-2">{t('audit.noFindings')}</p>
          <p className="text-sm text-text-muted">{t('audit.passed')}</p>
        </div>
      )}

      {pagePhase === 'completed' && liveStored?.createdAt && (
        <div className="mt-4 text-xs text-text-muted">
          KubeWise Security Scanner · {formatTimestamp(liveStored.createdAt)} ·{' '}
          {formatDuration(liveStored.result?.duration_ms ?? liveStored.result?.duration_ms)}
        </div>
      )}
    </div>
  );
}

function PhaseIcon({ status }: { status: string }) {
  if (status === 'completed') return <span className="text-green text-xs">✓</span>;
  if (status === 'running') {
    return <span className="w-2 h-2 rounded-full bg-accent animate-pulse" />;
  }
  if (status === 'failed') return <span className="text-red text-xs">✗</span>;
  return <span className="w-2 h-2 rounded-full border border-border" />;
}

function LiveFindingRow({ row }: { row: AuditFinding }) {
  return (
    <div className="flex items-start gap-2 text-sm px-3 py-2 rounded-sm bg-surface/60 border border-border/40">
      <span className={`text-xs font-semibold px-1.5 py-0.5 rounded-sm shrink-0 ${sevStyle[row.severity]}`}>
        {row.severity.toUpperCase()}
      </span>
      <span className="text-text-secondary shrink-0">{row.category}</span>
      <span className="font-mono text-text-muted truncate">{row.resource}</span>
    </div>
  );
}

function FindingsTable({ findings, dimmed }: { findings: AuditFinding[]; dimmed?: boolean }) {
  const { t } = useTranslation();
  if (findings.length === 0) {
    return (
      <div className="text-center py-12 text-sm text-text-muted border border-border rounded-sm">
        {t('audit.noMatch')}
      </div>
    );
  }

  return (
    <div
      className={`border border-border rounded-sm overflow-hidden transition-opacity ${
        dimmed ? 'opacity-80' : ''
      }`}
    >
      <table className="w-full text-sm">
        <thead>
          <tr className="bg-elevated/50">
            <th className="text-left text-xs text-text-muted font-semibold uppercase py-3 px-4">{t('audit.sev')}</th>
            <th className="text-left text-xs text-text-muted font-semibold uppercase py-3 px-4">{t('audit.category')}</th>
            <th className="text-left text-xs text-text-muted font-semibold uppercase py-3 px-4">{t('audit.resource')}</th>
            <th className="text-left text-xs text-text-muted font-semibold uppercase py-3 px-4">{t('audit.risk')}</th>
            <th className="text-left text-xs text-text-muted font-semibold uppercase py-3 px-4">{t('audit.impact')}</th>
            <th className="text-left text-xs text-text-muted font-semibold uppercase py-3 px-4">{t('audit.suggestion')}</th>
          </tr>
        </thead>
        <tbody>
          {findings.map((row, i) => (
            <tr
              key={`${row.resource}-${row.risk}-${i}`}
              className="border-t border-border/30 hover:bg-hover/30 transition-colors animate-fade-in"
            >
              <td className="py-3.5 px-4">
                <span className={`text-xs font-semibold px-2 py-0.5 rounded-sm ${sevStyle[row.severity]}`}>
                  {row.severity.toUpperCase()}
                </span>
              </td>
              <td className="py-3.5 px-4 text-text-secondary">{row.category}</td>
              <td className="py-3.5 px-4 font-mono text-text-muted text-sm">{row.resource}</td>
              <td className="py-3.5 px-4 font-mono text-text-muted text-sm">{row.risk}</td>
              <td className="py-3.5 px-4 text-text-muted text-sm">{row.impact}</td>
              <td className="py-3.5 px-4 text-accent text-sm font-medium">{row.suggestion}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
