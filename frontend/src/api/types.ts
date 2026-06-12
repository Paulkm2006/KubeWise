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
  status: 'pending' | 'running' | 'completed' | 'failed' | 'cancelled';
  created_at: string;
}

export interface DiagnosisListResponse {
  diagnoses: DiagnosisSummary[];
  total: number;
}

export interface FixAction {
  priority?: string;
  type?: string;
  description: string;
  command?: string;
  risk?: string;
}

export interface Evidence {
  id?: string;
  source?: string;
  signal?: string;
  strength?: string;
  summary?: string;
  detail?: string;
  num?: number;
  text?: string;
}

export interface DiagnosisDetail extends DiagnosisSummary {
  root_cause?: string;
  confidence?: string;
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

export interface DiagnosisEvent {
  diagnosis_id: string;
  seq_num: number;
  event_type: string;
  message?: string;
  summary?: string;
  detail?: string;
  payload_kind?: string;
  payload_json?: string;
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

export type DiagnosisVerdict = 'confirmed' | 'likely' | 'inconclusive';

export interface DiagnosisRootCause {
  category?: string;
  title?: string;
  confidence_score?: number;
  confidence_label?: string;
  summary: string;
}

export interface StructuredEvidence {
  id: string;
  source?: string;
  signal?: string;
  strength?: string;
  summary: string;
  detail?: string;
  raw_excerpt?: string;
}

export interface StructuredHypothesis {
  id: string;
  category?: string;
  title: string;
  status: string;
  rationale?: string;
  supporting_evidence?: string[];
  refuting_evidence?: string[];
}

export interface DiagnosisAction {
  priority?: string;
  description: string;
  command?: string;
  risk?: string;
}

export interface DiagnosisImpact {
  severity?: string;
  description: string;
}

export interface DiagnosisEnrichment {
  status: 'full' | 'degraded';
  degraded_steps?: string[];
  message?: string;
}

export interface DiagnosisResult {
  verdict: DiagnosisVerdict;
  root_cause: DiagnosisRootCause;
  evidence?: StructuredEvidence[];
  hypotheses?: StructuredHypothesis[];
  actions?: DiagnosisAction[];
  impact?: DiagnosisImpact;
  limitations?: string[];
  enrichment?: DiagnosisEnrichment;
  markdown?: string;
  duration_ms?: number;
}

export type DiagnosisLifecycleStatus =
  | 'pending'
  | 'running'
  | 'completed'
  | 'failed'
  | 'cancelled';

export interface DiagnosisStatus {
  diagnosis_id: string;
  status: DiagnosisLifecycleStatus;
  created_at?: string;
  target: DiagnosisTarget;
  events: DiagnosisEvent[];
  result?: DiagnosisResult;
}

export const DIAGNOSIS_STAGES = [
  { id: 'intake', label: 'Intake', detail: 'Normalize target context' },
  { id: 'collect', label: 'Collect', detail: 'Gather pod state, events, logs' },
  { id: 'analyze', label: 'Analyze', detail: 'Build evidence and hypotheses' },
  { id: 'verify', label: 'Verify', detail: 'Check hypotheses against evidence' },
  { id: 'report', label: 'Report', detail: 'Assemble structured diagnosis' },
] as const;

export type DiagnosisStageId = (typeof DIAGNOSIS_STAGES)[number]['id'];

export interface DiagnosisStageState {
  id: DiagnosisStageId;
  status: 'pending' | 'running' | 'completed';
  summary?: string;
}

export interface DiagnosisDerivedState {
  stages: DiagnosisStageState[];
  currentStage?: DiagnosisStageId;
  toolEvents: DiagnosisEvent[];
  liveEvidenceCount: number;
  liveHypothesisCount: number;
}

export type AuditSeverity = 'critical' | 'high' | 'medium' | 'low';

export interface AuditFinding {
  severity: AuditSeverity;
  category: string;
  resource: string;
  risk: string;
  impact: string;
  suggestion: string;
}

export interface AuditSummary {
  total: number;
  critical: number;
  high: number;
  medium: number;
  low: number;
}

export interface AuditResult {
  findings: AuditFinding[];
  summary: AuditSummary;
  markdown?: string;
  duration_ms?: number;
}

export type AuditLifecycleStatus = 'running' | 'completed' | 'failed' | 'cancelled';

export interface AuditTarget {
  cluster: string;
  cluster_display: string;
}

export interface AuditEvent {
  audit_id: string;
  seq_num: number;
  event_type: string;
  message?: string;
  summary?: string;
  detail?: string;
  payload_kind?: string;
  payload_json?: string;
  elapsed_ms?: number;
  created_at?: number;
}

export interface AuditStatus {
  audit_id: string;
  status: AuditLifecycleStatus;
  created_at?: string;
  target: AuditTarget;
  events: AuditEvent[];
  result?: AuditResult;
  error_message?: string;
}

export interface AuditSummaryRow {
  id: string;
  cluster_fingerprint: string;
  cluster_display?: string;
  status: AuditLifecycleStatus;
  created_at: string;
}

export interface AuditListResponse {
  audits: AuditSummaryRow[];
  total: number;
}

export const AUDIT_PHASES = [
  { id: 'rbac', label: 'RBAC' },
  { id: 'pod_security', label: 'Pod Security' },
  { id: 'network_policies', label: 'Network Policies' },
  { id: 'image_security', label: 'Image Security' },
] as const;

export type AuditPhaseId = (typeof AUDIT_PHASES)[number]['id'];

export interface AuditPhaseState {
  id: AuditPhaseId;
  label: string;
  status: 'pending' | 'running' | 'completed' | 'failed';
  count?: number;
  elapsedMs?: number;
  summary?: string;
}

export interface AuditDerivedState {
  phases: AuditPhaseState[];
  currentPhase?: AuditPhaseId;
  liveFindings: AuditFinding[];
  completedCount: number;
  totalPhases: number;
}
