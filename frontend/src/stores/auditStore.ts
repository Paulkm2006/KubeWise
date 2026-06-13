import {
  AUDIT_PHASES,
  type AuditDerivedState,
  type AuditEvent,
  type AuditFinding,
  type AuditPhaseId,
  type AuditResult,
  type AuditStatus,
  type AuditTarget,
} from '../api/types';
import { subscribeAudit, fetchAuditStatus } from '../api/sse';

export type StoredAuditStatus = 'running' | 'completed' | 'failed' | 'cancelled';

export interface StoredAudit {
  id: string;
  target: AuditTarget;
  events: AuditEvent[];
  lastSeqNum: number;
  status: StoredAuditStatus;
  createdAt?: string;
  result?: AuditResult;
  errorMessage?: string;
  derived: AuditDerivedState;
  cleanup: (() => void) | null;
}

type StoreCallbacks = {
  onUpdate: () => void;
  onTerminal?: (stored: StoredAudit) => void;
};

export class AuditStore {
  private audits: Map<string, StoredAudit> = new Map();

  get(id: string): StoredAudit | undefined {
    return this.audits.get(id);
  }

  getForCluster(cluster: string): StoredAudit | undefined {
    for (const a of this.audits.values()) {
      if (a.target.cluster === cluster) {
        return a;
      }
    }
    return undefined;
  }

  findRunning(cluster: string): StoredAudit | undefined {
    for (const a of this.audits.values()) {
      if (a.target.cluster === cluster && a.status === 'running') {
        return a;
      }
    }
    return undefined;
  }

  add(id: string, target: AuditTarget, callbacks: StoreCallbacks): StoredAudit {
    const stored: StoredAudit = {
      id,
      target,
      events: [],
      lastSeqNum: 0,
      status: 'running',
      derived: initialDerivedState(),
      cleanup: null,
    };
    this.audits.set(id, stored);
    this.connectSSE(id, 0, callbacks);
    return stored;
  }

  restoreFromStatus(status: AuditStatus, callbacks: StoreCallbacks): StoredAudit {
    const stored = this.buildStored(status);
    this.audits.set(stored.id, stored);
    if (stored.status === 'running') {
      this.connectSSE(stored.id, stored.lastSeqNum, callbacks);
    }
    return stored;
  }

  private buildStored(status: AuditStatus): StoredAudit {
    const events = status.events || [];
    const normalizedStatus = normalizeStatus(status.status);
    return {
      id: status.audit_id,
      target: status.target,
      events,
      lastSeqNum: events.length ? Math.max(...events.map((e) => e.seq_num)) : 0,
      status: normalizedStatus,
      createdAt: status.created_at,
      result: status.result,
      errorMessage: status.error_message,
      derived: foldAuditEvents(events),
      cleanup: null,
    };
  }

  private connectSSE(id: string, since: number, callbacks: StoreCallbacks): void {
    const stored = this.audits.get(id);
    if (!stored) return;

    stored.cleanup?.();
    stored.cleanup = subscribeAudit(id, since, {
      onEvent: (ev) => {
        stored.events.push(ev);
        stored.lastSeqNum = Math.max(stored.lastSeqNum, ev.seq_num);
        stored.derived = foldAuditEvents(stored.events);
        callbacks.onUpdate();
      },
      onComplete: async (status) => {
        stored.status = normalizeStatus(status);
        stored.cleanup = null;
        try {
          const full = await fetchAuditStatus(id);
          stored.status = normalizeStatus(full.status);
          if (full.created_at) stored.createdAt = full.created_at;
          if (full.result) stored.result = full.result;
          if (full.error_message) stored.errorMessage = full.error_message;
          if (full.events?.length) {
            stored.events = full.events;
            stored.lastSeqNum = Math.max(...full.events.map((e) => e.seq_num));
            stored.derived = foldAuditEvents(stored.events);
          }
        } catch {
          /* keep stream-derived state */
        }
        if (stored.status === 'completed' || stored.status === 'failed') {
          callbacks.onTerminal?.(stored);
        }
        callbacks.onUpdate();
      },
      onError: () => {
        stored.cleanup = null;
      },
    });
  }

  closeSSE(id: string): void {
    const stored = this.audits.get(id);
    if (stored?.cleanup) {
      stored.cleanup();
      stored.cleanup = null;
    }
  }
}

function normalizeStatus(status: string): StoredAuditStatus {
  if (status === 'completed' || status === 'failed' || status === 'cancelled') {
    return status;
  }
  return 'running';
}

function initialDerivedState(): AuditDerivedState {
  return {
    phases: AUDIT_PHASES.map((p) => ({
      id: p.id,
      label: p.label,
      status: 'pending' as const,
    })),
    liveFindings: [],
    completedCount: 0,
    totalPhases: AUDIT_PHASES.length,
  };
}

export function foldAuditEvents(events: AuditEvent[]): AuditDerivedState {
  const phases = initialDerivedState().phases.map((p) => ({ ...p }));
  const phaseIndex = new Map(phases.map((p, i) => [p.id, i]));
  let currentPhase: AuditPhaseId | undefined;
  const liveFindings: AuditFinding[] = [];
  let completedCount = 0;

  for (const ev of events) {
    if (ev.event_type === 'phase_start') {
      const phaseId = ev.message as AuditPhaseId | undefined;
      if (!phaseId) continue;
      const idx = phaseIndex.get(phaseId);
      if (idx === undefined) continue;
      for (let i = 0; i < idx; i++) {
        if (phases[i].status !== 'completed') phases[i].status = 'completed';
      }
      phases[idx].status = 'running';
      phases[idx].summary = ev.summary || ev.message;
      currentPhase = phaseId;
      continue;
    }

    if (ev.event_type === 'phase_done') {
      const phaseId = ev.message as AuditPhaseId | undefined;
      const payload = parsePayload(ev.payload_json);
      const idx = phaseId ? phaseIndex.get(phaseId) : undefined;
      if (idx !== undefined) {
        phases[idx].status = 'completed';
        phases[idx].summary = ev.summary;
        if (typeof payload?.count === 'number') phases[idx].count = payload.count;
        if (typeof payload?.elapsed_ms === 'number') phases[idx].elapsedMs = payload.elapsed_ms;
        completedCount = Math.max(completedCount, idx + 1);
        currentPhase = phaseId;
      }
      const findings = payload?.findings;
      if (Array.isArray(findings)) {
        for (const f of findings) {
          if (f && typeof f === 'object') {
            liveFindings.push(f as AuditFinding);
          }
        }
      }
      continue;
    }

    if (ev.event_type === 'phase_fail') {
      const phaseId = ev.message as AuditPhaseId | undefined;
      const idx = phaseId ? phaseIndex.get(phaseId) : undefined;
      if (idx !== undefined) {
        phases[idx].status = 'failed';
        phases[idx].summary = ev.detail || 'Failed';
      }
      continue;
    }

    if (ev.event_type === 'audit_complete') {
      for (const p of phases) {
        if (p.status !== 'failed') p.status = 'completed';
      }
      completedCount = phases.length;
      const payload = parsePayload(ev.payload_json);
      const findings = payload?.findings;
      if (Array.isArray(findings) && findings.length > 0) {
        liveFindings.length = 0;
        for (const f of findings) {
          if (f && typeof f === 'object') liveFindings.push(f as AuditFinding);
        }
      }
    }
  }

  return {
    phases,
    currentPhase,
    liveFindings,
    completedCount,
    totalPhases: AUDIT_PHASES.length,
  };
}

function parsePayload(raw?: string): Record<string, unknown> | null {
  if (!raw) return null;
  try {
    const parsed = JSON.parse(raw) as unknown;
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed as Record<string, unknown>;
    }
    return null;
  } catch {
    return null;
  }
}
