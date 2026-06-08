import { useState, useCallback } from 'react';
import Header from './components/Header';
import Sidebar from './components/Sidebar';
import Dashboard from './components/Dashboard';
import SecurityAudit from './components/SecurityAudit';
import Chat from './components/Chat';
import DiagnosisOverlay from './components/DiagnosisOverlay';
import { initialActivities, Activity } from './data/mock';

const TAB_CONFIG = [
  { id: 'dashboard', label: 'Dashboard', icon: '◈' },
  { id: 'audit', label: 'Audit', icon: '◇' },
  { id: 'chat', label: 'Chat', icon: '○' },
] as const;

export default function App() {
  const [activeTab, setActiveTab] = useState('dashboard');
  const [activeCluster, setActiveCluster] = useState('prod-us');
  const [activities, setActivities] = useState<Activity[]>(initialActivities);
  const [diagnosedPods, setDiagnosedPods] = useState<Set<string>>(new Set());
  const [diagOpen, setDiagOpen] = useState(false);
  const [diagTarget, setDiagTarget] = useState({ cluster: '', namespace: '', pod: '' });

  const addActivity = useCallback((type: Activity['type'], text: string, cluster?: string) => {
    const now = new Date();
    const time = `${now.getHours().toString().padStart(2, '0')}:${now.getMinutes().toString().padStart(2, '0')}`;
    setActivities((prev) => [{ type, text, cluster, time }, ...prev.slice(0, 19)]);
  }, []);

  const handleTabChange = (tab: string) => {
    setActiveTab(tab);
    if (tab === 'audit') addActivity('done', 'Security audit report ready');
  };

  const handleClusterChange = (name: string) => {
    setActiveCluster(name);
    addActivity('info', `Switched to ${name}`, name);
  };

  const handleDiagnose = (cluster: string, namespace: string, pod: string) => {
    setDiagTarget({ cluster, namespace, pod });
    setDiagOpen(true);
  };

  const handleDiagClose = () => {
    setDiagOpen(false);
  };

  const handleDiagDone = () => {
    const key = `${diagTarget.cluster}/${diagTarget.namespace}/${diagTarget.pod}`;
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
        onClusterChange={handleClusterChange}
        onActivity={addActivity}
      />

      <div className="flex flex-1 min-h-0">
        <Sidebar activities={activities} activeCluster={activeCluster} />

        <div className="w-px bg-border shrink-0" />

        <main className="flex-1 min-w-0 bg-surface">
          {activeTab === 'dashboard' && (
            <Dashboard
              activeCluster={activeCluster}
              onClusterChange={handleClusterChange}
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
        cluster={diagTarget.cluster}
        namespace={diagTarget.namespace}
        pod={diagTarget.pod}
        onClose={handleDiagClose}
        onActivity={(type, text, cluster) => {
          addActivity(type as Activity['type'], text, cluster);
          if (type === 'done') handleDiagDone();
        }}
      />
    </div>
  );
}
