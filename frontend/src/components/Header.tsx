import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { api } from '../api/client';
import type { ClusterSummary } from '../api/types';

const healthColors: Record<string, string> = {
  healthy: 'bg-green',
  warning: 'bg-amber',
  error: 'bg-red',
  info: 'bg-accent',
};

// Map API health values to color keys
const healthMap: Record<string, string> = {
  healthy: 'healthy',
  degraded: 'warning',
  offline: 'error',
};

interface Tab { id: string; label: string; icon: string }

interface HeaderProps {
  activeTab: string;
  onTabChange: (tab: string) => void;
  tabs: readonly Tab[];
  activeCluster: string;
  onClusterChange: (name: string) => void;
  onActivity: (type: string, text: string, cluster?: string) => void;
  onRefresh: () => void;
}

export default function Header({ activeTab, onTabChange, tabs, activeCluster, onClusterChange, onActivity, onRefresh }: HeaderProps) {
  const { t, i18n } = useTranslation();
  const [open, setOpen] = useState(false);
  const [freshness, setFreshness] = useState(0);
  const [clusters, setClusters] = useState<ClusterSummary[]>([]);

  const toggleLanguage = () => {
    const next = i18n.language === 'zh' ? 'en' : 'zh';
    i18n.changeLanguage(next);
  };

  // Fetch real cluster list from API
  useEffect(() => {
    api.clusters.list().then((data) => {
      setClusters(data);
      // Auto-select first cluster on initial load
      if (!activeCluster && data.length > 0) {
        onClusterChange(data[0].name);
      }
    }).catch(() => {});
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    const t = setInterval(() => setFreshness((s) => s + 1), 1000);
    return () => clearInterval(t);
  }, []);

  const active = clusters.find((c) => c.name === activeCluster) || clusters[0];
  const freshnessText = freshness === 0 ? t('header.justNow') : t('header.timeAgo', { seconds: freshness });

  return (
    <header className="h-14 flex items-center px-6 border-b border-border bg-surface shrink-0 select-none gap-4">
      {/* Brand */}
      <span className="text-lg font-semibold tracking-wide" style={{ color: '#d4a030' }}>
        KubeWise
      </span>

      {/* Tab nav */}
      <nav className="flex items-center gap-1 ml-6">
        {tabs.map((tab) => (
          <button
            key={tab.id}
            onClick={() => onTabChange(tab.id)}
            className={`px-4 py-2 text-sm font-medium rounded-sm transition-colors cursor-pointer
              ${activeTab === tab.id
                ? 'text-accent bg-accent-dim/30'
                : 'text-text-muted hover:text-text hover:bg-hover'
              }`}
          >
            {tab.icon} {tab.label}
          </button>
        ))}
      </nav>

      <div className="flex-1" />

      {/* Live indicator */}
      <span className="flex items-center gap-1.5 text-sm text-text-muted">
        <span className="relative flex w-2 h-2">
          <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-green opacity-50" />
          <span className="relative inline-flex rounded-full w-2 h-2 bg-green" />
        </span>
        {t('header.live')}
      </span>
      <span className="text-sm text-text-muted">{freshnessText}</span>

      {/* Refresh */}
      <button
        onClick={() => { onRefresh(); setFreshness(0); onActivity('info', t('header.refreshed'), activeCluster); }}
        className="text-sm text-text-muted px-3 py-1.5 border border-border hover:border-accent/30 hover:text-text rounded-sm transition-colors cursor-pointer bg-transparent"
      >
        {t('header.refresh')}
      </button>

      {/* Language Switcher */}
      <button
        onClick={toggleLanguage}
        className="text-sm text-text-muted px-3 py-1.5 border border-border hover:border-accent/30 hover:text-text rounded-sm transition-colors cursor-pointer bg-transparent"
      >
        {i18n.language === 'zh' ? 'EN' : '中'}
      </button>

      {/* Cluster Switcher */}
      <div className="relative" onMouseLeave={() => setOpen(false)}>
        <button
          onClick={() => setOpen((v) => !v)}
          className="flex items-center gap-2 px-3 py-1.5 text-sm text-text-secondary
                     border border-border hover:border-accent/40 rounded-sm transition-colors cursor-pointer bg-transparent"
        >
          <span className={`w-2 h-2 rounded-full ${healthColors[active ? healthMap[active.health] || 'info' : 'info']}`} />
          <span>{activeCluster}</span>
          <span className="text-text-muted">▾</span>
        </button>

        {open && (
          <>
            <div className="fixed inset-0 z-10" onClick={() => setOpen(false)} />
            <div className="absolute top-full right-0 mt-1.5 min-w-[200px] z-20 bg-elevated border border-border rounded-sm shadow-xl overflow-hidden">
              {clusters.map((c) => (
                <button
                  key={c.name}
                  onClick={() => { onClusterChange(c.name); setOpen(false); }}
                  className={`w-full flex items-center gap-2.5 px-4 py-3 text-left text-sm transition-colors cursor-pointer
                    border-b border-border/30 last:border-0
                    ${c.name === activeCluster ? 'text-accent bg-accent-dim/20' : 'text-text-secondary hover:bg-hover hover:text-text'}`}
                >
                  <span className={`w-2 h-2 rounded-full shrink-0 ${healthColors[healthMap[c.health] || 'info']}`} />
                  <span className="font-medium flex-1">{c.name}</span>
                  <span className="text-xs text-text-muted font-mono">{c.pods_ready}/{c.pods_total}</span>
                </button>
              ))}
            </div>
          </>
        )}
      </div>
    </header>
  );
}
