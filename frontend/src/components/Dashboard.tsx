import { useMemo, useState, useEffect, useCallback } from 'react';
import { api } from '../api/client';
import type { ClusterSummary, Issue, DiagnosisSummary } from '../api/types';

interface DashboardProps {
  activeCluster: string;
  onClusterChange: (name: string) => void;
  onDiagnose: (cluster: string, namespace: string, pod: string) => void;
  diagnosedPods: Set<string>;
}

const PAGE_SIZE = 10;

const healthDot: Record<string, string> = {
  healthy: 'bg-green',
  degraded: 'bg-amber',
  offline: 'bg-red',
};

const severityBadge: Record<string, string> = {
  high: 'text-red bg-red-dim',
  medium: 'text-amber bg-amber-dim',
  low: 'text-accent bg-accent-dim',
};

const severityRank: Record<string, number> = {
  high: 0,
  medium: 1,
  low: 2,
};

export default function Dashboard({ activeCluster, onClusterChange, onDiagnose, diagnosedPods }: DashboardProps) {
  const [page, setPage] = useState(1);
  const [clusters, setClusters] = useState<ClusterSummary[]>([]);
  const [allIssues, setAllIssues] = useState<Issue[]>([]);
  const [diagnoses, setDiagnoses] = useState<DiagnosisSummary[]>([]);
  const [diagnosingPods, setDiagnosingPods] = useState<Set<string>>(new Set());
  const [filterCluster, setFilterCluster] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Fetch clusters
  const fetchClusters = useCallback(async () => {
    try {
      const data = await api.clusters.list();
      setClusters(data);
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to fetch clusters');
    }
  }, []);

  // Fetch issues for a specific cluster
  const fetchIssuesForCluster = useCallback(async (name: string): Promise<Issue[]> => {
    try {
      return await api.clusters.issues(name);
    } catch {
      return [];
    }
  }, []);

  // Fetch all data on interval
  useEffect(() => {
    let mounted = true;

    const fetchAll = async () => {
      setLoading(true);
      try {
        const clusterData = await api.clusters.list();
        if (!mounted) return;
        setClusters(clusterData);
        setError(null);

        // Fetch issues for all clusters (or just filtered one)
        const names = filterCluster ? [filterCluster] : clusterData.map((c) => c.name);
        const results = await Promise.allSettled(names.map((n) => fetchIssuesForCluster(n)));
        if (!mounted) return;

        const all: Issue[] = [];
        results.forEach((r) => {
          if (r.status === 'fulfilled') all.push(...r.value);
        });
        setAllIssues(all);

        // Fetch recent diagnoses
        const diagData = await api.diagnoses.list({ limit: 10 });
        if (!mounted) return;
        setDiagnoses(diagData);
      } catch (e) {
        if (!mounted) return;
        setError(e instanceof Error ? e.message : 'Failed to fetch data');
      } finally {
        if (mounted) setLoading(false);
      }
    };

    fetchAll();
    const interval = setInterval(fetchAll, 15000);
    return () => { mounted = false; clearInterval(interval); };
  }, [filterCluster, fetchIssuesForCluster]);

  // Sort issues by severity (high > medium > low)
  const sortedIssues = useMemo(() => {
    return [...allIssues].sort((a, b) => severityRank[a.severity] - severityRank[b.severity]);
  }, [allIssues]);

  const totalPages = Math.max(1, Math.ceil(sortedIssues.length / PAGE_SIZE));
  const pagedIssues = sortedIssues.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE);

  const currentCluster = clusters.find((c) => c.name === activeCluster);

  // Reset page on filter or data change
  useEffect(() => { setPage(1); }, [filterCluster, sortedIssues.length]);

  const handleClusterClick = (name: string) => {
    if (filterCluster === name) {
      setFilterCluster(null);
    } else {
      setFilterCluster(name);
    }
    onClusterChange(name);
  };

  const handleDiagnoseClick = async (cluster: string, namespace: string, pod: string) => {
    const key = `${cluster}/${namespace}/${pod}`;
    if (diagnosedPods.has(key) || diagnosingPods.has(key)) return;
    setDiagnosingPods((prev) => new Set(prev).add(key));
    try {
      await onDiagnose(cluster, namespace, pod);
    } finally {
      setDiagnosingPods((prev) => {
        const next = new Set(prev);
        next.delete(key);
        return next;
      });
    }
  };

  return (
    <div className="h-full overflow-y-auto">
      {/* Clusters */}
      <div className="px-8 pt-6 pb-5 border-b border-border/60">
        <div className="flex items-center gap-3 mb-4">
          <h2 className="text-sm font-semibold text-text tracking-wide">Clusters</h2>
          {loading ? (
            <span className="text-sm text-text-muted font-mono">loading...</span>
          ) : (
            <span className="text-sm text-text-muted font-mono">{clusters.length} connected</span>
          )}
          {error && <span className="text-xs text-red ml-2">{error}</span>}
        </div>
        <div className="flex gap-3 overflow-x-auto scrollbar-hide pb-1">
          {clusters.map((c) => {
            const isActive = c.name === activeCluster;
            const isFiltered = filterCluster === c.name;
            return (
              <button
                key={c.fingerprint || c.name}
                onClick={() => handleClusterClick(c.name)}
                className={`group flex-shrink-0 w-52 text-left rounded-sm transition-all duration-150 cursor-pointer
                  ${isActive || isFiltered
                    ? 'bg-accent-dim/15 border-l-[3px] border-accent pl-[13px] pr-4 pt-4 pb-4'
                    : 'bg-surface border border-border hover:bg-elevated p-4'
                  }`}
              >
                <div className="flex items-center justify-between">
                  <span className={`text-sm font-semibold ${isActive || isFiltered ? 'text-accent' : 'text-text'}`}>
                    {c.name}
                  </span>
                  <span className={`w-2.5 h-2.5 rounded-full ${healthDot[c.health] || 'bg-text-muted'} shrink-0`} />
                </div>
                <div className="mt-2 flex items-center justify-between">
                  <span className="text-sm text-text-secondary font-mono">
                    <span className={
                      c.pods_ready === c.pods_total ? 'text-green' : c.issues_count > 0 ? 'text-red' : 'text-amber'
                    }>
                      ●
                    </span>{' '}
                    {c.pods_ready}/{c.pods_total} pods ready
                  </span>
                  <span className="text-xs text-text-muted font-mono">{c.last_updated}s</span>
                </div>
                <div className="mt-1.5">
                  {c.issues_count > 0 ? (
                    <span className={`text-sm font-medium ${c.issues_count > 2 ? 'text-red' : 'text-amber'}`}>
                      {c.issues_count} issue{c.issues_count > 1 ? 's' : ''}
                    </span>
                  ) : (
                    <span className="text-sm text-text-muted">0 issues</span>
                  )}
                </div>
                <div className={`mt-3 pt-3 border-t border-border/30 flex gap-3 text-xs text-text-muted ${isActive || isFiltered ? '' : 'opacity-0 group-hover:opacity-100 transition-opacity'}`}>
                  <span>◈ {c.nodes} nodes</span>
                  <span>▣ {c.namespaces} namespaces</span>
                </div>
              </button>
            );
          })}
          {clusters.length === 0 && !loading && (
            <div className="text-sm text-text-muted py-4">No clusters found. Check connection.</div>
          )}
        </div>
        {filterCluster && (
          <div className="mt-3 flex items-center gap-2 animate-fade-in">
            <span className="text-xs text-text-muted">Filtered by:</span>
            <span className="text-xs text-accent font-medium px-2 py-0.5 rounded-sm bg-accent-dim/15 border border-accent/20">
              {filterCluster}
            </span>
            <button
              onClick={() => setFilterCluster(null)}
              className="text-xs text-text-muted hover:text-text cursor-pointer bg-transparent border-none transition-colors"
            >
              ✕ clear
            </button>
          </div>
        )}
      </div>

      {/* Issues + side content */}
      <div className="px-8 py-6 grid grid-cols-[1.5fr_1fr] gap-8 items-stretch">
        {/* Issues */}
        <div className="flex flex-col">
          <div className="flex items-center justify-between mb-4 shrink-0">
            <div className="flex items-center gap-3">
              <h2 className="text-sm font-semibold text-text tracking-wide">Issues</h2>
              <span className="text-sm text-red/80 font-mono font-medium">
                {sortedIssues.length > 0
                  ? `${sortedIssues.length} total${filterCluster ? ` on ${filterCluster}` : ' across all clusters'}`
                  : 'none'}
              </span>
            </div>

            {/* Pagination */}
            {totalPages > 1 && (
              <div className="flex items-center gap-1.5">
                <button
                  onClick={() => setPage((p) => Math.max(1, p - 1))}
                  disabled={page === 1}
                  className="text-xs text-text-muted px-2 py-1 rounded-sm border border-border
                             hover:border-accent/30 hover:text-accent disabled:opacity-30 disabled:cursor-not-allowed
                             transition-colors cursor-pointer bg-transparent"
                >
                  ‹
                </button>
                {Array.from({ length: totalPages }, (_, i) => i + 1).map((p) => (
                  <button
                    key={p}
                    onClick={() => setPage(p)}
                    className={`text-xs px-2 py-1 rounded-sm border transition-colors cursor-pointer min-w-[28px] text-center
                      ${p === page
                        ? 'border-accent/40 text-accent bg-accent-dim/15'
                        : 'border-border text-text-muted hover:text-text hover:bg-hover bg-transparent'
                      }`}
                  >
                    {p}
                  </button>
                ))}
                <button
                  onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                  disabled={page === totalPages}
                  className="text-xs text-text-muted px-2 py-1 rounded-sm border border-border
                             hover:border-accent/30 hover:text-accent disabled:opacity-30 disabled:cursor-not-allowed
                             transition-colors cursor-pointer bg-transparent"
                >
                  ›
                </button>
              </div>
            )}
          </div>

          {loading && clusters.length === 0 ? (
            <div className="border border-border rounded-sm p-10 text-center">
              <p className="text-sm text-text-muted">Loading issues...</p>
            </div>
          ) : sortedIssues.length === 0 ? (
            <div className="border border-border rounded-sm p-10 text-center">
              <p className="text-sm text-text-muted">No issues</p>
              <p className="text-xs text-text-muted mt-1">All clusters are running normally</p>
            </div>
          ) : (
            <div key={`issues-${filterCluster || 'all'}-${page}`} className="border border-border rounded-sm overflow-hidden flex-1 flex flex-col animate-fade-in">
              <table className="w-full">
                <thead>
                  <tr className="bg-elevated/50">
                    <th className="text-left text-xs text-text-muted font-semibold uppercase py-3 px-4">Sev</th>
                    <th className="text-left text-xs text-text-muted font-semibold uppercase py-3 px-4">Cluster</th>
                    <th className="text-left text-xs text-text-muted font-semibold uppercase py-3 px-4">Pod</th>
                    <th className="text-left text-xs text-text-muted font-semibold uppercase py-3 px-4">Status</th>
                    <th className="text-left text-xs text-text-muted font-semibold uppercase py-3 px-4">Ns</th>
                    <th className="text-right text-xs text-text-muted font-semibold uppercase py-3 px-4">Action</th>
                  </tr>
                </thead>
                <tbody>
                  {pagedIssues.map((iss, i) => {
                    const isDone = diagnosedPods.has(`${iss.cluster}/${iss.namespace}/${iss.pod}`);
                    const isDiagnosing = diagnosingPods.has(`${iss.cluster}/${iss.namespace}/${iss.pod}`);
                    return (
                      <tr key={`${iss.cluster}/${iss.namespace}/${iss.pod}-${i}`} className="group border-t border-border/30 hover:bg-hover/50 transition-colors">
                        <td className="py-3 px-4">
                          <span className={`text-xs font-semibold px-2 py-0.5 rounded-sm ${severityBadge[iss.severity]}`}>
                            {iss.severity.toUpperCase()}
                          </span>
                        </td>
                        <td className="py-3 px-4 text-sm text-text-secondary font-mono">{iss.cluster}</td>
                        <td className="py-3 px-4 text-sm font-medium text-text">{iss.pod}</td>
                        <td className="py-3 px-4 text-sm text-text-secondary font-mono">{iss.status}</td>
                        <td className="py-3 px-4 text-sm text-text-muted font-mono">{iss.namespace}</td>
                        <td className="py-3 px-4 text-right">
                          <button
                            onClick={() => handleDiagnoseClick(iss.cluster, iss.namespace, iss.pod)}
                            disabled={isDone || isDiagnosing}
                            className={`text-xs font-medium px-3 py-1.5 rounded-sm border transition-colors cursor-pointer
                              ${isDone
                                ? 'border-green/30 text-green bg-green-dim/10'
                                : isDiagnosing
                                ? 'border-accent/30 text-accent bg-accent-dim/10'
                                : 'border-border hover:text-accent hover:border-accent/40 text-text-muted bg-transparent'
                              }`}
                          >
                            {isDone ? '✓ Done' : isDiagnosing ? '⟳ ...' : 'Diagnose →'}
                          </button>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </div>

        {/* Right side */}
        <div className="space-y-6">
          {/* Recent Diagnoses (replaces Quick Actions) */}
          {diagnoses.length > 0 && (
            <div className="border border-border rounded-sm p-5 bg-surface">
              <h3 className="text-xs text-text-muted font-semibold uppercase tracking-wider mb-3">Recent Diagnoses</h3>
              <div className="space-y-2 max-h-[240px] overflow-y-auto">
                {diagnoses.slice(0, 10).map((d) => (
                  <div key={d.id} className="flex items-start gap-2 px-3 py-2 rounded-sm hover:bg-hover/50 transition-colors">
                    <span className={`mt-1 w-1.5 h-1.5 rounded-full shrink-0 ${d.resolved ? 'bg-green' : 'bg-amber'}`} />
                    <div className="min-w-0 flex-1">
                      <p className="text-sm text-text-secondary leading-snug truncate">
                        <span className="font-mono text-xs text-text-muted">{d.cluster_display || d.cluster_fingerprint.slice(0, 8)}</span>
                        {' '}{d.pod}
                      </p>
                      <p className="text-xs text-text-muted truncate mt-0.5">{d.root_cause || 'Pending...'}</p>
                      <p className="text-[10px] text-text-muted font-mono mt-0.5">{d.created_at}</p>
                    </div>
                    <span className={`text-[10px] px-1.5 py-0.5 rounded-sm font-mono shrink-0 ${d.confidence === 'high' ? 'text-green bg-green-dim/20' : 'text-amber bg-amber-dim/20'}`}>
                      {d.confidence}
                    </span>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Stats — reflects current cluster */}
          {currentCluster ? (
            <div className="grid grid-cols-2 gap-3">
              {[
                { label: 'Pods', value: `${currentCluster.pods_ready}/${currentCluster.pods_total}`, color: 'text-text' },
                { label: 'Issues', value: currentCluster.issues_count, color: currentCluster.issues_count > 0 ? 'text-red' : 'text-text' },
                { label: 'Nodes', value: currentCluster.nodes, color: 'text-text' },
                { label: 'Namespaces', value: currentCluster.namespaces, color: 'text-text' },
              ].map((s) => (
                <div key={s.label} className="border border-border rounded-sm p-4 text-center bg-surface hover:bg-elevated transition-colors cursor-pointer">
                  <p className={`text-2xl font-semibold font-mono ${s.color}`}>{s.value}</p>
                  <p className="text-sm text-text-muted mt-1">{s.label}</p>
                </div>
              ))}
            </div>
          ) : (
            <div className="border border-border rounded-sm p-6 text-center bg-surface">
              <p className="text-sm text-text-muted">No cluster data</p>
            </div>
          )}

          {/* Version info */}
          {currentCluster && (
            <div className="border border-border rounded-sm p-4 bg-surface">
              <p className="text-xs text-text-muted font-semibold uppercase tracking-wider mb-2">Cluster Info</p>
              <div className="space-y-1">
                <div className="flex justify-between text-xs">
                  <span className="text-text-muted">Version</span>
                  <span className="text-text-secondary font-mono">{currentCluster.version || 'N/A'}</span>
                </div>
                <div className="flex justify-between text-xs">
                  <span className="text-text-muted">Fingerprint</span>
                  <span className="text-text-secondary font-mono text-[10px]">{currentCluster.fingerprint.slice(0, 16)}...</span>
                </div>
              </div>
            </div>
          )}
        </div>
      </div>

      <div className="px-8 pb-6">
        <p className="text-xs text-text-muted border-t border-border/30 pt-4">
          CIS Kubernetes Benchmark v1.10 · NIST SP 800-204 · Auto-refresh 15s
        </p>
      </div>
    </div>
  );
}
