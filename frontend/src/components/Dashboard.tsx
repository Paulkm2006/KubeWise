import { useMemo, useState, useEffect } from 'react';
import { clusters, issues, resources, ClusterHealth } from '../data/mock';

interface DashboardProps {
  activeCluster: string;
  onClusterChange: (name: string) => void;
  onDiagnose: (cluster: string, namespace: string, pod: string) => void;
  diagnosedPods: Set<string>;
}

const PAGE_SIZE = 10;

const healthDot: Record<ClusterHealth, string> = {
  healthy: 'bg-green',
  warning: 'bg-amber',
  error: 'bg-red',
  info: 'bg-accent',
};

const severityBadge: Record<string, string> = {
  high: 'text-red bg-red-dim',
  medium: 'text-amber bg-amber-dim',
  low: 'text-accent bg-accent-dim',
};

const resourceColor: Record<string, string> = {
  high: 'text-red',
  medium: 'text-amber',
  low: 'text-accent',
  ok: 'text-text-muted',
};

export default function Dashboard({ activeCluster, onClusterChange, onDiagnose, diagnosedPods }: DashboardProps) {
  const [page, setPage] = useState(1);

  const sortedClusters = clusters;

  const currentCluster = clusters.find((c) => c.id === activeCluster)!;

  const filteredIssues = useMemo(() => {
    return issues.filter((i) => i.cluster === activeCluster);
  }, [activeCluster]);

  const totalPages = Math.max(1, Math.ceil(filteredIssues.length / PAGE_SIZE));
  const pagedIssues = filteredIssues.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE);

  // Reset page when switching cluster
  useEffect(() => { setPage(1); }, [activeCluster]);

  return (
    <div className="h-full overflow-y-auto">
      {/* Clusters */}
      <div className="px-8 pt-6 pb-5 border-b border-border/60">
        <div className="flex items-center gap-3 mb-4">
          <h2 className="text-sm font-semibold text-text tracking-wide">Clusters</h2>
          <span className="text-sm text-text-muted font-mono">{clusters.length} connected</span>
        </div>
        <div className="flex gap-3 overflow-x-auto scrollbar-hide pb-1">
          {sortedClusters.map((c) => {
            const isActive = c.id === activeCluster;
            return (
              <button
                key={c.id}
                onClick={() => onClusterChange(c.id)}
                className={`group flex-shrink-0 w-52 text-left rounded-sm transition-all duration-150 cursor-pointer
                  ${isActive
                    ? 'bg-accent-dim/15 border-l-[3px] border-accent pl-[13px] pr-4 pt-4 pb-4'
                    : 'bg-surface border border-border hover:bg-elevated p-4'
                  }`}
              >
                <div className="flex items-center justify-between">
                  <span className={`text-sm font-semibold ${isActive ? 'text-accent' : 'text-text'}`}>
                    {c.name}
                  </span>
                  <span className={`w-2.5 h-2.5 rounded-full ${healthDot[c.health]} shrink-0`} />
                </div>
                <div className="mt-2 flex items-center justify-between">
                  <span className="text-sm text-text-secondary font-mono">
                    <span className={c.podsReady === c.podsTotal ? 'text-green' : c.issues > 0 ? 'text-red' : 'text-amber'}>●</span>{' '}
                    {c.podsReady}/{c.podsTotal} pods ready
                  </span>
                  <span className="text-xs text-text-muted font-mono">{c.lastUpdated}s</span>
                </div>
                <div className="mt-1.5">
                  {c.issues > 0 ? (
                    <span className={`text-sm font-medium ${c.issues > 2 ? 'text-red' : 'text-amber'}`}>
                      {c.issues} issue{c.issues > 1 ? 's' : ''}
                    </span>
                  ) : (
                    <span className="text-sm text-text-muted">0 issues</span>
                  )}
                </div>
                <div className={`mt-3 pt-3 border-t border-border/30 flex gap-3 text-xs text-text-muted ${isActive ? '' : 'opacity-0 group-hover:opacity-100 transition-opacity'}`}>
                  <span>◈ {c.nodes} nodes</span>
                  <span>▣ {c.namespaces} namespaces</span>
                </div>
              </button>
            );
          })}
        </div>
      </div>

      {/* Issues + side content — stretch both columns to match height */}
      <div className="px-8 py-6 grid grid-cols-[1.5fr_1fr] gap-8 items-stretch">
        {/* Issues */}
        <div className="flex flex-col">
          <div className="flex items-center justify-between mb-4 shrink-0">
            <div className="flex items-center gap-3">
              <h2 className="text-sm font-semibold text-text tracking-wide">Issues</h2>
              <span className="text-sm text-red/80 font-mono font-medium">
                {filteredIssues.length > 0 ? `${filteredIssues.length} on ${activeCluster}` : 'none'}
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

          {filteredIssues.length === 0 ? (
            <div className="border border-border rounded-sm p-10 text-center">
              <p className="text-sm text-text-muted">No issues on {activeCluster}</p>
              <p className="text-xs text-text-muted mt-1">All pods are running normally</p>
            </div>
          ) : (
            <div className="border border-border rounded-sm overflow-hidden flex-1 flex flex-col">
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
                    return (
                      <tr key={i} className="group border-t border-border/30 hover:bg-hover/50 transition-colors">
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
                            onClick={() => !isDone && onDiagnose(iss.cluster, iss.namespace, iss.pod)}
                            className={`text-xs font-medium px-3 py-1.5 rounded-sm border transition-colors cursor-pointer
                              ${isDone
                                ? 'border-green/30 text-green bg-green-dim/10'
                                : 'border-border hover:text-accent hover:border-accent/40 text-text-muted bg-transparent'
                              }`}
                          >
                            {isDone ? '✓ Done' : 'Diagnose →'}
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
          {/* Quick Actions */}
          <div className="border border-border rounded-sm p-5 bg-surface">
            <h3 className="text-xs text-text-muted font-semibold uppercase tracking-wider mb-4">Quick</h3>
            <div className="space-y-2">
              {[
                { label: 'Security Audit', icon: '◇' },
                { label: 'Resource Top', icon: '▤' },
                { label: 'Open Chat', icon: '○' },
              ].map((a) => (
                <button key={a.label} className="w-full flex items-center gap-3 px-3 py-2.5 text-sm text-text-muted hover:text-text hover:bg-hover rounded-sm transition-colors cursor-pointer bg-transparent">
                  <span>{a.icon}</span>
                  {a.label}
                </button>
              ))}
            </div>
          </div>

          {/* Stats — reflects current cluster */}
          <div className="grid grid-cols-2 gap-3">
            {[
              { label: 'Pods', value: `${currentCluster.podsReady}/${currentCluster.podsTotal}`, color: 'text-text' },
              { label: 'Issues', value: currentCluster.issues, color: currentCluster.issues > 0 ? 'text-red' : 'text-text' },
              { label: 'Nodes', value: currentCluster.nodes, color: 'text-text' },
              { label: 'Namespaces', value: currentCluster.namespaces, color: 'text-text' },
            ].map((s) => (
              <div key={s.label} className="border border-border rounded-sm p-4 text-center bg-surface hover:bg-elevated transition-colors cursor-pointer">
                <p className={`text-2xl font-semibold font-mono ${s.color}`}>{s.value}</p>
                <p className="text-sm text-text-muted mt-1">{s.label}</p>
              </div>
            ))}
          </div>

          {/* Resource Table — filters to current cluster */}
          <div>
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-xs text-text-muted font-semibold uppercase tracking-wider">Resources ({activeCluster})</h3>
              <span className="text-xs text-text-muted font-mono">updated 4s ago</span>
            </div>
            <div className="border border-border rounded-sm overflow-hidden">
              <table className="w-full">
                <thead>
                  <tr className="bg-elevated/50">
                    <th className="text-left text-xs text-text-muted font-semibold uppercase py-2.5 px-3">Pod</th>
                    <th className="text-right text-xs text-text-muted font-semibold uppercase py-2.5 px-3">CPU</th>
                    <th className="text-right text-xs text-text-muted font-semibold uppercase py-2.5 px-3">Mem</th>
                    <th className="text-right text-xs text-text-muted font-semibold uppercase py-2.5 px-3">Disk</th>
                  </tr>
                </thead>
                <tbody>
                  {resources.map((r, i) => (
                    <tr key={i} className="border-t border-border/30 hover:bg-hover/30 transition-colors">
                      <td className="py-2.5 px-3 text-sm text-text font-mono">{r.pod}</td>
                      <td className={`py-2.5 px-3 text-right text-sm font-mono ${resourceColor[r.cpuStatus]}`}>{r.cpu}</td>
                      <td className={`py-2.5 px-3 text-right text-sm font-mono ${resourceColor[r.memoryStatus]}`}>{r.memory}</td>
                      <td className="py-2.5 px-3 text-right text-sm font-mono text-text-muted">{r.disk}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      </div>

      <div className="px-8 pb-6">
        <p className="text-xs text-text-muted border-t border-border/30 pt-4">
          CIS Kubernetes Benchmark v1.10 · NIST SP 800-204 · 2026-06-07 08:15 UTC
        </p>
      </div>
    </div>
  );
}
