import { useState, useCallback, useRef, useEffect } from 'react';
import Header from './components/Header';
import Sidebar from './components/Sidebar';
import Dashboard from './components/Dashboard';
import SecurityAudit from './components/SecurityAudit';
import Chat from './components/Chat';
import DiagnosisOverlay from './components/DiagnosisOverlay';
import { Activity } from './data/mock';
import { api } from './api/client';
import { DiagnosisStore, StoredDiagnosis } from './stores/diagnosisStore';
import { AuditStore } from './stores/auditStore';
import type { DiagnosisTarget, ClusterSummary } from './api/types';

const TAB_CONFIG = [
  { id: 'dashboard', label: '仪表盘', icon: '◈' },
  { id: 'audit', label: '安全审计', icon: '◇' },
  { id: 'chat', label: '对话', icon: '○' },
] as const;

type DiagPhase = 'idle' | 'loading' | 'ready' | 'error';

function podKey(cluster: string, namespace: string, pod: string) {
  return `${cluster}/${namespace}/${pod}`;
}

export default function App() {
  const [activeTab, setActiveTab] = useState('dashboard');
  const [activeCluster, setActiveCluster] = useState('');
  const [focusCluster, setFocusCluster] = useState<string | null>(null);
  const [activities, setActivities] = useState<Activity[]>([]);
  const [diagnosedPods, setDiagnosedPods] = useState<Set<string>>(new Set());
  const [diagOpen, setDiagOpen] = useState(false);
  const [diagPhase, setDiagPhase] = useState<DiagPhase>('idle');
  const [diagError, setDiagError] = useState<string | null>(null);
  const [diagTarget, setDiagTarget] = useState({ cluster: '', namespace: '', pod: '' });
  const storeRef = useRef(new DiagnosisStore());
  const auditStoreRef = useRef(new AuditStore());
  const [activeDiagnosis, setActiveDiagnosis] = useState<StoredDiagnosis | null>(null);
  const [, forceUpdate] = useState(0);
  const [clusters, setClusters] = useState<ClusterSummary[]>([]);
  const [refreshKey, setRefreshKey] = useState(0);
  const handleRefresh = useCallback(() => setRefreshKey((k) => k + 1), []);

  useEffect(() => {
    api.clusters.list().then(setClusters).catch(() => {});
  }, []);

  const addActivity = useCallback((type: string, text: string, cluster?: string) => {
    const now = new Date();
    const time = `${now.getHours().toString().padStart(2, '0')}:${now.getMinutes().toString().padStart(2, '0')}`;
    setActivities((prev) => [{ type: type as Activity['type'], text, cluster, time }, ...prev.slice(0, 19)]);
  }, []);

  const storeCallbacks = useCallback(
    () => ({
      onUpdate: () => forceUpdate((n) => n + 1),
      onTerminal: (stored: StoredDiagnosis) => {
        const key = podKey(stored.target.cluster, stored.target.namespace, stored.target.pod);
        setDiagnosedPods((prev) => new Set(prev).add(key));
        addActivity(
          stored.status === 'failed' ? 'issue' : 'done',
          `${stored.target.pod} 诊断${stored.status === 'completed' ? '完成' : stored.status === 'failed' ? '失败' : stored.status}`,
          stored.target.cluster,
        );
      },
    }),
    [addActivity],
  );

  const clearDoneBadge = useCallback((cluster: string, namespace: string, pod: string) => {
    const key = podKey(cluster, namespace, pod);
    setDiagnosedPods((prev) => {
      if (!prev.has(key)) return prev;
      const next = new Set(prev);
      next.delete(key);
      return next;
    });
  }, []);

  const handleTabChange = (tab: string) => {
    setActiveTab(tab);
  };

  const handleClusterChange = (name: string) => {
    setActiveCluster(name);
    addActivity('info', `已切换到 ${name}`, name);
  };

  const openDiagnosisById = async (id: string) => {
    setDiagOpen(true);
    setDiagPhase('loading');
    setDiagError(null);
    setActiveDiagnosis(null);

    try {
      const status = await api.diagnoses.get(id);
      const { cluster, namespace, pod } = status.target;
      clearDoneBadge(cluster, namespace, pod);
      setDiagTarget({ cluster, namespace, pod });
      const stored = storeRef.current.restoreFromStatus(status, storeCallbacks());
      setActiveDiagnosis(stored);
      setDiagPhase('ready');
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Unknown error';
      setDiagPhase('error');
      setDiagError(msg);
      addActivity('issue', `打开诊断失败: ${msg}`);
    }
  };

  const openDiagnosisForPod = async (cluster: string, namespace: string, pod: string) => {
    clearDoneBadge(cluster, namespace, pod);
    setDiagTarget({ cluster, namespace, pod });
    setDiagOpen(true);
    setDiagPhase('loading');
    setDiagError(null);
    setActiveDiagnosis(null);

    const target: DiagnosisTarget = { cluster, cluster_display: cluster, namespace, pod };
    const callbacks = storeCallbacks();

    try {
      const latest = await api.diagnoses.latest({ cluster, namespace, pod });
      const stored = storeRef.current.restoreFromStatus(latest, callbacks);
      setActiveDiagnosis(stored);
      setDiagPhase('ready');
      return;
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Unknown error';
      if (!msg.includes('404')) {
        setDiagPhase('error');
        setDiagError(msg);
        addActivity('issue', `${pod} 诊断查询失败: ${msg}`, cluster);
        return;
      }
    }

    try {
      addActivity('pending', `正在诊断 ${pod}...`, cluster);
      const res = await api.diagnoses.create(cluster, namespace, pod);
      const stored = storeRef.current.add(res.diagnosis_id, target, callbacks);
      setActiveDiagnosis(stored);
      setDiagPhase('ready');
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Unknown error';
      setDiagPhase('error');
      setDiagError(msg);
      addActivity('issue', `${pod} 诊断失败: ${msg}`, cluster);
    }
  };

  const handleRerun = async () => {
    if (!activeDiagnosis) return;
    const { cluster, namespace, pod } = activeDiagnosis.target;
    clearDoneBadge(cluster, namespace, pod);
    setDiagPhase('loading');
    setDiagError(null);

    const target: DiagnosisTarget = {
      cluster,
      cluster_display: activeDiagnosis.target.cluster_display || cluster,
      namespace,
      pod,
    };
    const callbacks = storeCallbacks();

    try {
      if (activeDiagnosis.status === 'running') {
        await api.diagnoses.cancel(activeDiagnosis.id);
        storeRef.current.closeSSE(activeDiagnosis.id);
      }

      addActivity('pending', `重新诊断 ${pod}...`, cluster);
      const res = await api.diagnoses.create(cluster, namespace, pod);
      const stored = storeRef.current.add(res.diagnosis_id, target, callbacks);
      setActiveDiagnosis(stored);
      setDiagPhase('ready');
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Unknown error';
      setDiagPhase('error');
      setDiagError(msg);
      addActivity('issue', `${pod} 重新诊断失败: ${msg}`, cluster);
    }
  };

  const handleDiagClose = () => {
    setDiagOpen(false);
    setDiagPhase('idle');
    setDiagError(null);
  };

  return (
    <div className="h-screen flex flex-col bg-bg min-w-0">
      <div className="h-[3px] shrink-0 bg-accent" />

      <Header
        activeTab={activeTab}
        onTabChange={handleTabChange}
        tabs={TAB_CONFIG}
        activeCluster={activeCluster}
        onClusterChange={handleClusterChange}
        onActivity={addActivity}
        onRefresh={handleRefresh}
      />

      <div className="flex flex-1 min-h-0">
        <Sidebar
          activities={activities}
          activeCluster={activeCluster}
          clusters={clusters}
          onClear={() => setActivities([])}
          onActivityClick={(cluster) => { if (cluster) { setActiveCluster(cluster); addActivity('info', `已切换到 ${cluster}`, cluster); } }}
          onFocusCluster={(name) => { setFocusCluster(name); setActiveTab('dashboard'); }}
        />

        <div className="w-px bg-border shrink-0" />

        <main className="flex-1 min-w-0 bg-surface">
          {activeTab === 'dashboard' && (
            <Dashboard
              activeCluster={activeCluster}
              focusCluster={focusCluster}
              onClusterChange={handleClusterChange}
              onFocusChange={setFocusCluster}
              onDiagnose={openDiagnosisForPod}
              onOpenDiagnosis={openDiagnosisById}
              diagnosedPods={diagnosedPods}
              refreshKey={refreshKey}
            />
          )}
          {activeTab === 'audit' && (
            <SecurityAudit
              activeCluster={activeCluster}
              store={auditStoreRef.current}
              onActivity={addActivity}
              onStoreUpdate={() => forceUpdate((n) => n + 1)}
              refreshKey={refreshKey}
            />
          )}
          {activeTab === 'chat' && <Chat activeCluster={activeCluster} />}
        </main>
      </div>

      <DiagnosisOverlay
        open={diagOpen}
        phase={diagPhase}
        error={diagError}
        target={diagTarget}
        diagnosis={activeDiagnosis}
        onClose={handleDiagClose}
        onRerun={handleRerun}
      />
    </div>
  );
}
