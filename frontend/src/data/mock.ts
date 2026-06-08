export type ClusterHealth = 'healthy' | 'warning' | 'error' | 'info';

export interface Cluster {
  id: string;
  name: string;
  health: ClusterHealth;
  podsReady: number;
  podsTotal: number;
  issues: number;
  nodes: number;
  namespaces: number;
  lastUpdated: number;
}

export interface Issue {
  severity: 'high' | 'medium' | 'low';
  cluster: string;
  pod: string;
  status: string;
  namespace: string;
}

export interface Activity {
  type: 'done' | 'issue' | 'pending' | 'info';
  text: string;
  cluster?: string;
  time: string;
}

export interface ResourceRow {
  pod: string;
  cpu: string;
  cpuStatus: 'high' | 'medium' | 'low' | 'ok';
  memory: string;
  memoryStatus: 'high' | 'medium' | 'low' | 'ok';
  disk: string;
}

export interface DiagnosisStep {
  id: number;
  label: string;
  detail: string;
}

export interface DiagnosisResult {
  rootCause: string;
  confidence: string;
  evidence: { num: number; text: string }[];
  fixCommand: string;
  impact: string;
  duration: string;
}

export interface ChatMessage {
  id: string;
  role: 'user' | 'ai';
  text: string;
  timestamp: string;
  cluster?: string;
}

export interface AuditFinding {
  severity: 'high' | 'medium' | 'low';
  category: string;
  resource: string;
  risk: string;
  impact: string;
  suggestion: string;
}

export const clusters: Cluster[] = [
  { id: 'prod-cn', name: 'prod-cn', health: 'healthy', podsReady: 12, podsTotal: 12, issues: 1, nodes: 8, namespaces: 32, lastUpdated: 3 },
  { id: 'prod-us', name: 'prod-us', health: 'error', podsReady: 8, podsTotal: 10, issues: 16, nodes: 10, namespaces: 28, lastUpdated: 3 },
  { id: 'staging', name: 'staging', health: 'healthy', podsReady: 5, podsTotal: 5, issues: 0, nodes: 3, namespaces: 12, lastUpdated: 3 },
  { id: 'dev', name: 'dev', health: 'info', podsReady: 3, podsTotal: 3, issues: 0, nodes: 1, namespaces: 8, lastUpdated: 3 },
  { id: 'prod-eu', name: 'prod-eu', health: 'healthy', podsReady: 6, podsTotal: 6, issues: 0, nodes: 6, namespaces: 24, lastUpdated: 8 },
  { id: 'prod-ap', name: 'prod-ap', health: 'error', podsReady: 2, podsTotal: 4, issues: 3, nodes: 4, namespaces: 16, lastUpdated: 5 },
];

export const issues: Issue[] = [
  { severity: 'high', cluster: 'prod-us', pod: 'nginx-7d9f', status: 'CrashLoopBackOff (12/12)', namespace: 'prod' },
  { severity: 'high', cluster: 'prod-us', pod: 'mysql-0', status: 'CrashLoopBackOff (8/8)', namespace: 'prod' },
  { severity: 'high', cluster: 'prod-us', pod: 'kafka-1', status: 'CrashLoopBackOff (6/6)', namespace: 'kafka' },
  { severity: 'high', cluster: 'prod-us', pod: 'etcd-2', status: 'OOMKilled (exit 137)', namespace: 'kube-system' },
  { severity: 'medium', cluster: 'prod-us', pod: 'redis-3a2b', status: 'Pending (insufficient cpu)', namespace: 'prod' },
  { severity: 'medium', cluster: 'prod-us', pod: 'api-gw-5f1a', status: 'OOMKilled (exit 137)', namespace: 'default' },
  { severity: 'medium', cluster: 'prod-us', pod: 'auth-svc-2c4b', status: 'CrashLoopBackOff (5/5)', namespace: 'default' },
  { severity: 'medium', cluster: 'prod-us', pod: 'cache-7d9x', status: 'ImagePullBackOff', namespace: 'default' },
  { severity: 'medium', cluster: 'prod-us', pod: 'worker-queue-1a', status: 'Pending (insufficient memory)', namespace: 'worker' },
  { severity: 'low', cluster: 'prod-us', pod: 'cron-cleanup-7d', status: 'Pending (evicted)', namespace: 'cron' },
  { severity: 'low', cluster: 'prod-us', pod: 'log-forwarder-x2', status: 'ImagePullBackOff', namespace: 'monitoring' },
  { severity: 'low', cluster: 'prod-us', pod: 'metric-collector-3f', status: 'CrashLoopBackOff (2/2)', namespace: 'monitoring' },
  { severity: 'low', cluster: 'prod-us', pod: 'dns-cache-9c1', status: 'OOMKilled (exit 137)', namespace: 'kube-system' },
  { severity: 'low', cluster: 'prod-us', pod: 'backup-job-4e', status: 'Error (backoff limit)', namespace: 'cron' },
  { severity: 'low', cluster: 'prod-us', pod: 'sidecar-injector-8a', status: 'Pending (node pressure)', namespace: 'kube-system' },
  { severity: 'low', cluster: 'prod-us', pod: 'fluentd-7b', status: 'CrashLoopBackOff (4/4)', namespace: 'monitoring' },
  { severity: 'high', cluster: 'prod-ap', pod: 'api-gw-6f3a', status: 'ImagePullBackOff', namespace: 'default' },
  { severity: 'medium', cluster: 'prod-ap', pod: 'worker-x1b2', status: 'OOMKilled (exit 137)', namespace: 'worker' },
  { severity: 'low', cluster: 'prod-ap', pod: 'cron-sync-9c', status: 'CrashLoopBackOff (3/3)', namespace: 'cron' },
  { severity: 'low', cluster: 'prod-cn', pod: 'log-agent-2d1', status: 'Pending (evicted)', namespace: 'monitoring' },
];

export const resources: ResourceRow[] = [
  { pod: 'nginx-7d9f', cpu: '1.2', cpuStatus: 'high', memory: '2.1', memoryStatus: 'high', disk: '50' },
  { pod: 'redis-3a2b', cpu: '0.9', cpuStatus: 'medium', memory: '1.0', memoryStatus: 'medium', disk: '30' },
  { pod: 'api-gw-5f1', cpu: '0.7', cpuStatus: 'low', memory: '1.5', memoryStatus: 'low', disk: '20' },
  { pod: 'auth-svc-2c4', cpu: '0.5', cpuStatus: 'ok', memory: '0.6', memoryStatus: 'ok', disk: '10' },
  { pod: 'worker-x9', cpu: '0.3', cpuStatus: 'ok', memory: '0.3', memoryStatus: 'ok', disk: '5' },
];

export const initialActivities: Activity[] = [
  { type: 'pending', text: 'Redis Pending detected', cluster: 'prod-us', time: '14:22' },
  { type: 'done', text: 'nginx-7d9f diagnosis: OOMKilled', cluster: 'prod-us', time: '14:20' },
  { type: 'done', text: 'Security audit: 12 findings', time: '14:15' },
  { type: 'issue', text: 'CrashLoopBackOff detected', cluster: 'prod-us', time: '14:10' },
];

export const diagnosisSteps: DiagnosisStep[] = [
  { id: 1, label: 'Collecting context', detail: 'Pod describe, events, logs...' },
  { id: 2, label: 'LLM Analysis', detail: 'Analyzing symptoms...' },
  { id: 3, label: 'Hypothesis verification', detail: 'Verifying root cause...' },
  { id: 4, label: 'Generating report', detail: 'Structuring findings...' },
];

export const mockDiagnosisResult: DiagnosisResult = {
  rootCause: 'OOMKilled — Memory limit insufficient',
  confidence: '92%',
  evidence: [
    { num: 1, text: 'Container exited with code 137 (OOMKilled confirmation)' },
    { num: 2, text: 'Memory limit = 64Mi, actual peak = 256Mi (4× over limit)' },
    { num: 3, text: 'Node status: 32Gi / 12Gi used — no resource pressure' },
  ],
  fixCommand: 'kubectl set resources deployment nginx \\\n  --container nginx \\\n  --limits memory=512Mi \\\n  -n prod',
  impact: 'Rolling restart, 1/3 replicas briefly unavailable (~30s)',
  duration: '28s',
};

export const auditFindings: AuditFinding[] = [
  { severity: 'high', category: 'Wildcard RBAC', resource: 'ClusterRole / cluster-admin', risk: 'verbs: ["*"], resources: ["*"]', impact: 'Any bound SA can perform any action cluster-wide', suggestion: 'Replace with scoped roles + specific verbs' },
  { severity: 'high', category: 'Default SA Overprivileged', resource: 'default / production', risk: 'Bound to edit ClusterRole', impact: 'Default SA has namespace-wide write access in production', suggestion: 'Remove binding or bind only necessary roles' },
  { severity: 'high', category: 'Privileged Container', resource: 'Deployment / datadog-agent', risk: 'privileged: true', impact: 'Container can access host kernel capabilities', suggestion: 'Use least-privileged securityContext' },
  { severity: 'medium', category: 'Privilege Escalation', resource: 'Role / ci-deployer', risk: 'verbs: ["escalate", "impersonate"]', impact: 'Allows privilege escalation attacks', suggestion: 'Remove escalate/impersonate verbs' },
  { severity: 'medium', category: 'Privileged Verb', resource: 'Role / pod-exec', risk: 'verbs: ["exec"]', impact: 'SA has pod exec access in 3 namespaces', suggestion: 'Restrict exec to specific namespaces only' },
  { severity: 'medium', category: 'Cross-ns Escalation', resource: 'ServiceAccount / monitor', risk: 'ClusterRole bound to ns-level SA', impact: 'SA can read resources across all namespaces', suggestion: 'Use Role + RoleBinding scoped to one namespace' },
  { severity: 'medium', category: 'Delete Permission', resource: 'Role / cleaner', risk: 'verbs: ["delete"]', impact: 'SA can delete pods in default namespace', suggestion: 'Limit delete to specific resource types' },
  { severity: 'low', category: 'Unused SA', resource: 'ServiceAccount / legacy-bot', risk: 'Orphaned SA with valid binding', impact: 'No pods using this SA, but binding exists', suggestion: 'Remove unused SA' },
  { severity: 'low', category: 'Unused SA', resource: 'ServiceAccount / old-cicd', risk: 'Orphaned SA with valid binding', impact: 'No pods using this SA, but binding exists', suggestion: 'Remove unused SA' },
  { severity: 'low', category: 'Automount Token', resource: 'Pod / default-editor', risk: 'automountServiceAccountToken: true (default)', impact: 'Default token mounted; unnecessary for this workload', suggestion: 'Set automountServiceAccountToken: false' },
];

export const chatResponses: Record<string, string> = {
  'Show all PVs by size': `Querying PersistentVolumes across all clusters...

| Rank | PV | Capacity | Bound To | Cluster |
|------|-----|----------|----------|----------|
| 1 | pvc-data-1 | 500Gi | mysql-0 | prod-us |
| 2 | pvc-logs-x | 200Gi | elastic-0 | prod-cn |
| 3 | pvc-www | 100Gi | www-7d9f | prod-us |
| 4 | pvc-cache | 50Gi | redis-3a | staging |
| 5 | pvc-temp | 10Gi | build-pod | dev |

Largest PV: pvc-data-1 (500Gi) → bound to mysql-0 in prod-us.`,

  'Deploy ArgoCD in dev': `Deploying ArgoCD in dev namespace...

**Phase 1/7** ✓ Check namespace existence
**Phase 2/7** ✓ Resolve chart via ChainResolver
**Phase 3/7** ◎ helm repo add argo-cd && helm show values
**Phase 4/7** ○ LLM generating override values
**Phase 5/7** ○ Awaiting user confirmation
**Phase 6/7** ○ helm install execution
**Phase 7/7** ○ Verify release and report`,

  'Diagnose nginx crash': `Starting diagnosis for nginx-7d9f in prod-us/prod...

**Step 1** — Collecting pod describe, events, logs... ✓
**Step 2** — LLM analyzing symptoms... ✓
**Step 3** — Verifying OOM hypothesis... ✓

**Root Cause**: OOMKilled — Memory limit insufficient
**Confidence**: 92%

→ kubectl set resources deployment nginx --limits memory=512Mi -n prod`,

  'Cluster resource usage': `Resource usage across all clusters (top consumers):

**prod-us** — CPU: 4.2/8.0 cores | Mem: 8.5/16 Gi
**prod-cn** — CPU: 3.1/8.0 cores | Mem: 6.2/16 Gi
**prod-eu** — CPU: 1.2/4.0 cores | Mem: 3.0/8 Gi
**staging** — CPU: 0.8/2.0 cores | Mem: 1.5/4 Gi
**dev** — CPU: 0.3/1.0 cores | Mem: 0.6/2 Gi`,
};
