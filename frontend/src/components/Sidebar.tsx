import { useTranslation } from 'react-i18next';
import type { ClusterSummary } from '../api/types';

interface SidebarProps {
  activities: Activity[];
  activeCluster: string;
  clusters: ClusterSummary[];
  onClear?: () => void;
  onActivityClick?: (activity: Activity) => void;
  onFocusCluster?: (name: string) => void;
}

interface Activity {
  type: 'done' | 'issue' | 'pending' | 'info';
  text: string;
  cluster?: string;
  time: string;
  kind?: 'diagnosis' | 'audit' | 'cluster_switch' | 'info';
  detail?: string;
}

const typeStyle: Record<string, { border: string; dot: string }> = {
  done: { border: 'border-l-green/50', dot: 'bg-green' },
  issue: { border: 'border-l-red/50', dot: 'bg-red' },
  pending: { border: 'border-l-amber/50', dot: 'bg-amber' },
  info: { border: 'border-l-accent/50', dot: 'bg-accent' },
};

export default function Sidebar({ activities, activeCluster, clusters, onClear, onActivityClick, onFocusCluster }: SidebarProps) {
  const { t } = useTranslation();
  const clusterInfo = clusters.find((c) => c.name === activeCluster);

  return (
    <aside className="w-56 shrink-0 flex flex-col bg-bg">
      {/* Activity Feed */}
      <div className="flex-1 flex flex-col min-h-0 p-4">
        <div className="flex items-center justify-between mb-3">
          <span className="text-xs font-semibold text-text-muted tracking-widest uppercase">{t('sidebar.activity')}</span>
          <span onClick={onClear} className="text-xs text-text-muted cursor-pointer hover:text-text transition-colors">{t('sidebar.clear')}</span>
        </div>
        <div className="flex-1 overflow-y-auto space-y-1">
          {activities.map((a, i) => {
            const s = typeStyle[a.type] || typeStyle.info;
            return (
              <div
                key={i}
                onClick={() => onActivityClick?.(a)}
                className={`flex items-start gap-2.5 px-3 py-2 border-l-2 ${s.border} hover:bg-hover/50 transition-colors rounded-r-sm ${a.cluster ? 'cursor-pointer' : 'cursor-default'}`}
              >
                <span className={`mt-1.5 w-1.5 h-1.5 rounded-full shrink-0 ${s.dot}`} />
                <div className="min-w-0 flex-1">
                  <p className="text-sm text-text-secondary leading-snug">{a.text}</p>
                  <p className="text-xs text-text-muted mt-0.5 font-mono">
                    {a.cluster ? `${a.cluster} · ` : ''}{a.time}
                  </p>
                </div>
              </div>
            );
          })}
        </div>
      </div>

      {/* Cluster State Card */}
      <div className="p-4 pt-2">
        {clusterInfo ? (
          <div
            onClick={() => onFocusCluster?.(activeCluster)}
            className="p-4 bg-elevated border border-border rounded-sm cursor-pointer hover:border-accent/30 transition-colors"
          >
            <div className="flex items-center gap-2">
              <span className={`w-2 h-2 rounded-full shrink-0 ${
                clusterInfo.health === 'healthy' ? 'bg-green' :
                clusterInfo.health === 'degraded' ? 'bg-amber' : 'bg-red'
              }`} />
              <p className="text-sm font-semibold text-text truncate">{clusterInfo.name}</p>
            </div>
            <div className="flex items-center gap-2 mt-2">
              <span className={`text-xs px-2 py-0.5 rounded-sm font-medium ${
                clusterInfo.issues_count > 0 ? 'bg-amber-dim text-amber' : 'bg-green-dim text-green'
              }`}>
                {t('sidebar.issues', { count: clusterInfo.issues_count })}
              </span>
              <span className="text-sm text-text-muted font-mono">
                {t('sidebar.podsReady', { ready: clusterInfo.pods_ready, total: clusterInfo.pods_total })}
              </span>
            </div>
            <div className="flex gap-3 mt-3 text-sm text-text-muted">
              <span>{t('sidebar.nodes', { count: clusterInfo.nodes })}</span>
              <span>{t('sidebar.namespaces', { count: clusterInfo.namespaces })}</span>
            </div>
          </div>
        ) : activeCluster ? (
          <div className="p-4 bg-elevated border border-border rounded-sm">
            <p className="text-sm text-text-muted">{activeCluster}</p>
            <p className="text-xs text-text-muted mt-2">{t('sidebar.loadingCluster')}</p>
          </div>
        ) : null}
      </div>
    </aside>
  );
}