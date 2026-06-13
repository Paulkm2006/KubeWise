export interface Activity {
  type: 'done' | 'issue' | 'pending' | 'info';
  text: string;
  cluster?: string;
  time: string;
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

export const diagnosisSteps: DiagnosisStep[] = [
  { id: 1, label: '采集上下文', detail: 'Pod describe、事件、日志...' },
  { id: 2, label: 'LLM 分析', detail: '分析症状...' },
  { id: 3, label: '假设验证', detail: '验证根因...' },
  { id: 4, label: '生成报告', detail: '结构化发现...' },
];

export const mockDiagnosisResult: DiagnosisResult = {
  rootCause: 'OOMKilled — 内存限制不足',
  confidence: '92%',
  evidence: [
    { num: 1, text: '容器以退出码 137 退出 (OOMKilled 确认)' },
    { num: 2, text: '内存限制 = 64Mi，实际峰值 = 256Mi (超出限制 4 倍)' },
    { num: 3, text: '节点状态: 32Gi / 12Gi 已用 — 无资源压力' },
  ],
  fixCommand: 'kubectl set resources deployment nginx \\\n  --container nginx \\\n  --limits memory=512Mi \\\n  -n prod',
  impact: '滚动重启，1/3 副本短暂不可用 (~30秒)',
  duration: '28秒',
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
