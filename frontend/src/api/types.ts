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
  cluster_display?: string;
  namespace: string;
  pod: string;
  status: string;
  created_at: string;
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

/** A single diagnosis event from the backend (maps to Go EventRecord) */
export interface DiagnosisEvent {
  diagnosis_id: string;
  seq_num: number;
  event_type: string;
  message?: string;
  detail?: string;
  token_in?: number;
  token_out?: number;
  elapsed_ms?: number;
  created_at?: number;
}

export interface DiagnosisTarget {
  cluster: string;
  cluster_display: string;
  namespace: string;
  pod: string;
}

export interface DiagnosisResult {
  root_cause: string;
  confidence?: string;
  evidence?: { num: number; text: string }[];
  fix_actions?: { type: string; description: string; command?: string; risk?: string }[];
  impact?: string;
  duration_ms?: number;
}

export interface DiagnosisStatus {
  diagnosis_id: string;
  status: 'running' | 'completed' | 'failed';
  target: DiagnosisTarget;
  events: DiagnosisEvent[];
  result?: DiagnosisResult;
}