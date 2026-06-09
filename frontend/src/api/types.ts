export interface ClusterSummary {
  id: string;
  name: string;
  health: 'healthy' | 'degraded' | 'offline';
  pods_ready: number;
  pods_total: number;
  issues_count: number;
  nodes: number;
  namespaces: number;
  version: string;
  fingerprint: string;
  last_updated: number;
}

export interface Issue {
  severity: 'high' | 'medium' | 'low';
  cluster: string;
  pod: string;
  namespace: string;
  status: string;
  restarts: number;
  age: string;
}

export interface DiagnosisSummary {
  id: string;
  cluster_fingerprint: string;
  cluster_display: string;
  disconnected: boolean;
  namespace: string;
  pod: string;
  root_cause: string;
  confidence: string;
  created_at: string;
  resolved: number;
}

export interface FixAction {
  type: string;
  description: string;
  command?: string;
  risk?: string;
}

export interface Evidence {
  num: number;
  text: string;
}

export interface DiagnosisDetail extends DiagnosisSummary {
  status: 'pending' | 'running' | 'completed' | 'failed';
  evidence?: Evidence[];
  fix_actions?: FixAction[];
  impact?: string;
  duration_ms?: number;
}

export interface Activity {
  id: string;
  type: 'diagnosis' | 'cluster_switch' | 'system';
  text: string;
  cluster_display?: string;
  diagnosis_id?: string;
  created_at: string;
}

export interface StreamEvent {
  type: string;
  message?: string;
  detail?: string;
}
