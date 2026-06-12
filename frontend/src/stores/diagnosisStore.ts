import {
  DIAGNOSIS_STAGES,
  type DiagnosisDerivedState,
  type DiagnosisEvent,
  type DiagnosisResult,
  type DiagnosisStageId,
  type DiagnosisStatus,
  type DiagnosisTarget,
} from '../api/types';
import { subscribeDiagnosis, fetchDiagnosisStatus } from '../api/sse';

export type StoredDiagnosisStatus = 'running' | 'completed' | 'failed' | 'cancelled';

export interface StoredDiagnosis {
  id: string;
  target: DiagnosisTarget;
  events: DiagnosisEvent[];
  lastSeqNum: number;
  status: StoredDiagnosisStatus;
  createdAt?: string;
  result?: DiagnosisResult;
  derived: DiagnosisDerivedState;
  cleanup: (() => void) | null;
}

type StoreCallbacks = {
  onUpdate: () => void;
  onTerminal?: (stored: StoredDiagnosis) => void;
};

export class DiagnosisStore {
  private diagnoses: Map<string, StoredDiagnosis> = new Map();

  get(id: string): StoredDiagnosis | undefined {
    return this.diagnoses.get(id);
  }

  findExisting(target: DiagnosisTarget): StoredDiagnosis | undefined {
    for (const d of this.diagnoses.values()) {
      if (
        d.target.cluster === target.cluster &&
        d.target.namespace === target.namespace &&
        d.target.pod === target.pod &&
        d.status === 'running'
      ) {
        return d;
      }
    }
    return undefined;
  }

  add(id: string, target: DiagnosisTarget, callbacks: StoreCallbacks): StoredDiagnosis {
    const stored: StoredDiagnosis = {
      id,
      target,
      events: [],
      lastSeqNum: 0,
      status: 'running',
      derived: initialDerivedState(),
      cleanup: null,
    };
    this.diagnoses.set(id, stored);
    this.connectSSE(id, 0, callbacks);
    return stored;
  }

  restoreFromStatus(status: DiagnosisStatus, callbacks: StoreCallbacks): StoredDiagnosis {
    const stored = this.buildStored(status);
    this.diagnoses.set(stored.id, stored);

    if (stored.status === 'running') {
      this.connectSSE(stored.id, stored.lastSeqNum, callbacks);
    }
    return stored;
  }

  async restore(id: string, callbacks: StoreCallbacks): Promise<void> {
    try {
      const status = await fetchDiagnosisStatus(id);
      this.restoreFromStatus(status, callbacks);
    } catch {
      // Server restart or unknown ID — will be created fresh
    }
  }

  private buildStored(status: DiagnosisStatus): StoredDiagnosis {
    const events = status.events || [];
    const normalizedStatus = normalizeStatus(status.status);
    return {
      id: status.diagnosis_id,
      target: status.target,
      events,
      lastSeqNum: events.length ? Math.max(...events.map((e) => e.seq_num)) : 0,
      status: normalizedStatus,
      createdAt: status.created_at,
      result: status.result,
      derived: foldDiagnosisEvents(events),
      cleanup: null,
    };
  }

  private connectSSE(id: string, since: number, callbacks: StoreCallbacks): void {
    const stored = this.diagnoses.get(id);
    if (!stored) return;

    stored.cleanup?.();

    stored.cleanup = subscribeDiagnosis(id, since, {
      onEvent: (ev) => {
        stored.events.push(ev);
        stored.lastSeqNum = Math.max(stored.lastSeqNum, ev.seq_num);
        stored.derived = foldDiagnosisEvents(stored.events);
        callbacks.onUpdate();
      },
      onComplete: async (status) => {
        if (
          status === 'completed' ||
          status === 'failed' ||
          status === 'cancelled' ||
          status === 'running'
        ) {
          stored.status = normalizeStatus(status);
        }
        stored.cleanup = null;
        try {
          const full = await fetchDiagnosisStatus(id);
          stored.status = normalizeStatus(full.status);
          if (full.created_at) stored.createdAt = full.created_at;
          if (full.result) stored.result = full.result;
          if (full.events?.length) {
            stored.events = full.events;
            stored.lastSeqNum = Math.max(...full.events.map((e) => e.seq_num));
            stored.derived = foldDiagnosisEvents(stored.events);
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
    const stored = this.diagnoses.get(id);
    if (stored?.cleanup) {
      stored.cleanup();
      stored.cleanup = null;
    }
  }

  setResult(
    id: string,
    result: DiagnosisResult,
    status: 'completed' | 'failed',
  ): void {
    const stored = this.diagnoses.get(id);
    if (stored) {
      stored.result = result;
      stored.status = status;
    }
  }

  remove(id: string): void {
    this.closeSSE(id);
    this.diagnoses.delete(id);
  }
}

function normalizeStatus(status: string): StoredDiagnosisStatus {
  if (status === 'completed' || status === 'failed' || status === 'cancelled') {
    return status;
  }
  return 'running';
}

function initialDerivedState(): DiagnosisDerivedState {
  return {
    stages: DIAGNOSIS_STAGES.map((s) => ({ id: s.id, status: 'pending' as const })),
    toolEvents: [],
    liveEvidenceCount: 0,
    liveHypothesisCount: 0,
  };
}

export function foldDiagnosisEvents(events: DiagnosisEvent[]): DiagnosisDerivedState {
  const stages = initialDerivedState().stages.map((s) => ({ ...s }));
  const stageIndex = new Map(stages.map((s, i) => [s.id, i]));
  let currentStage: DiagnosisStageId | undefined;
  let liveEvidenceCount = 0;
  let liveHypothesisCount = 0;

  const markCompletedThrough = (stageId: DiagnosisStageId) => {
    const idx = stageIndex.get(stageId);
    if (idx === undefined) return;
    for (let i = 0; i < idx; i++) {
      stages[i].status = 'completed';
    }
    stages[idx].status = 'completed';
  };

  for (const ev of events) {
    if (ev.event_type === 'tool_call' || ev.event_type === 'tool_done' || ev.event_type === 'tool_fail') {
      /* collected below */
    }

    const payload = parsePayload(ev.payload_json);
    if (ev.payload_kind === 'diagnosis_evidence' && payload && !Array.isArray(payload)) {
      const count = payload['evidence_count'];
      if (typeof count === 'number') liveEvidenceCount = count;
    }
    if (ev.payload_kind === 'diagnosis_hypothesis' && ev.payload_json) {
      try {
        const arr = JSON.parse(ev.payload_json) as unknown;
        if (Array.isArray(arr)) liveHypothesisCount = arr.length;
      } catch {
        /* ignore */
      }
    }

    if (ev.event_type !== 'phase') continue;

    const stagePayload =
      ev.payload_kind === 'diagnosis_stage' && payload && !Array.isArray(payload)
        ? payload
        : null;
    const stageFromPayload = stagePayload?.['stage'];
    const stageId =
      (typeof stageFromPayload === 'string' ? (stageFromPayload as DiagnosisStageId) : undefined) ??
      phaseToStage(ev.message);
    if (!stageId) continue;

    const idx = stageIndex.get(stageId);
    if (idx === undefined) continue;

    const summary = ev.summary || ev.message;
    if (stagePayload?.['status'] === 'completed') {
      markCompletedThrough(stageId);
      stages[idx].summary = summary;
      currentStage = stageId;
      continue;
    }

    for (let i = 0; i < idx; i++) {
      if (stages[i].status !== 'completed') stages[i].status = 'completed';
    }
    stages[idx].status = 'running';
    stages[idx].summary = summary;
    currentStage = stageId;
  }

  if (events.some((e) => e.event_type === 'agent_done')) {
    for (const s of stages) {
      s.status = 'completed';
    }
    currentStage = 'report';
  }

  const toolEvents = events.filter(
    (e) =>
      e.event_type === 'tool_call' ||
      e.event_type === 'tool_done' ||
      e.event_type === 'tool_fail',
  );

  return {
    stages,
    currentStage,
    toolEvents,
    liveEvidenceCount,
    liveHypothesisCount,
  };
}

function phaseToStage(phase?: string): DiagnosisStageId | undefined {
  if (!phase) return undefined;
  const normalized = phase.replace(/^diagnosis\./, '');
  if (normalized === 'format' || normalized === 'done') return 'report';
  if (normalized === 'evidence' || normalized === 'hypothesis') return 'analyze';
  if (['intake', 'collect', 'analyze', 'verify', 'report'].includes(normalized)) {
    return normalized as DiagnosisStageId;
  }
  return undefined;
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

export { DIAGNOSIS_STAGES as diagnosisStageMeta };
