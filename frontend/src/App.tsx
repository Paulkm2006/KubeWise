import { useState, useCallback, useRef } from 'react';
import Header from './components/Header';
import Sidebar from './components/Sidebar';
import Dashboard from './components/Dashboard';
import SecurityAudit from './components/SecurityAudit';
import Chat from './components/Chat';
import DiagnosisOverlay from './components/DiagnosisOverlay';
import { initialActivities, Activity } from './data/mock';
import { api } from './api/client';
import { DiagnosisStore, StoredDiagnosis } from './stores/diagnosisStore';
import type { DiagnosisTarget } from './api/types';

const TAB_CONFIG = [
  { id: 'dashboard', label: 'Dashboard', icon: '◈' },
  { id: 'audit', label: 'Audit', icon: '◇' },
  { id: 'chat', label: 'Chat', icon: '○' },
] as const;

export default function App() {
  const [activeTab, setActiveTab] = useState('dashboard');
  const [activeCluster, setActiveCluster] = useState('');
  const [focusCluster, setFocusCluster] = useState<string | null>(null);
  const [refreshKey, setRefreshKey] = useState(0);
  const [activities, setActivities] = useState<Activity[]>(initialActivities);
  const [diagnosedPods, setDiagnosedPods] = useState<Set<string>>(new Set());
  const [diagOpen, setDiagOpen] = useState(false);
  const [diagTarget, setDiagTarget] = useState({ cluster: '', namespace: '', pod: '' });
  const storeRef = useRef(new DiagnosisStore());
  const [activeDiagnosis, setActiveDiagnosis] = useState<StoredDiagnosis | null>(null);
  const [, forceUpdate] = useState(0);

  const addActivity = useCallback((type: string, text: string, cluster?: string) => {
    const now = new Date();
    const time = `${now.getHours().toString().padStart(2, '0')}:${now.getMinutes().toString().padStart(2, '0')}`;
    setActivities((prev) => [{ type: type as Activity['type'], text, cluster, time }, ...prev.slice(0, 19)]);
  }, []);

  const handleTabChange = (tab: string) => {
    setActiveTab(tab);
    if (tab === 'audit') addActivity('done', 'Security audit report ready');
  };

  const handleClusterChange = (name: string) => {
    setActiveCluster(name);
    addActivity('info', `Switched to ${name}`, name);
  };

  const handleRefresh = () => {
    setRefreshKey((k) => k + 1);
    addActivity('info', 'Views refreshed', activeCluster);
  };

  const handleDiagnose = async (cluster: string, namespace: string, pod: string) => {
    addActivity('pending', `Diagnosing ${pod}...`, cluster);

    const target: DiagnosisTarget = { cluster, cluster_display: cluster, namespace, pod };
    setDiagTarget({ cluster, namespace, pod });

    // Check if we already have a running diagnosis for this pod
    const existing = storeRef.current.findExisting(target);
    if (existing) {
      setActiveDiagnosis(existing);
      setDiagOpen(true);
      return;
    }

    try {
      const res = await api.diagnoses.create(cluster, namespace, pod);
      storeRef.current.add(res.diagnosis_id, target, () => forceUpdate(n => n + 1));
      setActiveDiagnosis(storeRef.current.get(res.diagnosis_id) || null);
      setDiagOpen(true);
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Unknown error';
      addActivity('issue', `Diagnosis failed for ${pod}: ${msg}`, cluster);
    }
  };

  const handleDiagClose = () => {
    setDiagOpen(false);
    // SSE stays alive in store — don't close it
  };

  const handleDiagDone = () => {
    if (!activeDiagnosis) return;
    const key = `${activeDiagnosis.target.cluster}/${activeDiagnosis.target.namespace}/${activeDiagnosis.target.pod}`;
    setDiagnosedPods((prev) => new Set(prev).add(key));
  };

  return (
    <div className="h-screen flex flex-col bg-bg min-w-0">
      <div className="h-[3px] shrink-0 bg-accent" />

      <Header
        activeTab={activeTab}
        onTabChange={handleTabChange}
        tabs={TAB_CONFIG}
        activeCluster={activeCluster}
        onRefresh={handleRefresh}
        onClusterChange={handleClusterChange}
      />

      <div className="flex flex-1 min-h-0">
        <Sidebar activities={activities} activeCluster={activeCluster} />

        <div className="w-px bg-border shrink-0" />

        <main className="flex-1 min-w-0 bg-surface">
          {activeTab === 'dashboard' && (
            <Dashboard
              activeCluster={activeCluster}
              focusCluster={focusCluster}
              onClusterChange={handleClusterChange}
              onFocusChange={setFocusCluster}
              onDiagnose={handleDiagnose}
              diagnosedPods={diagnosedPods}
            />
          )}
          {activeTab === 'audit' && <SecurityAudit />}
          {activeTab === 'chat' && <Chat />}
        </main>
      </div>

      <DiagnosisOverlay
        open={diagOpen}
        diagnosis={activeDiagnosis}
        onClose={handleDiagClose}
        onActivity={(type, text, cluster) => {
          addActivity(type, text, cluster);
          if (type === 'done') handleDiagDone();
        }}
      />
    </div>
  );
}