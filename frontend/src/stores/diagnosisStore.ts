import type { DiagnosisEvent, DiagnosisResult, DiagnosisTarget } from '../api/types';
import { subscribeDiagnosis, fetchDiagnosisStatus } from '../api/sse';

export interface StoredDiagnosis {
  id: string;
  target: DiagnosisTarget;
  events: DiagnosisEvent[];
  lastSeqNum: number;
  status: 'running' | 'completed' | 'failed';
  result?: DiagnosisResult;
  cleanup: (() => void) | null;
}

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

  add(id: string, target: DiagnosisTarget, onUpdate: () => void): void {
    const stored: StoredDiagnosis = {
      id,
      target,
      events: [],
      lastSeqNum: 0,
      status: 'running',
      cleanup: null,
    };
    this.diagnoses.set(id, stored);
    this.connectSSE(id, 0, onUpdate);
  }

  async restore(id: string, onUpdate: () => void): Promise<void> {
    try {
      const status = await fetchDiagnosisStatus(id);
      const stored: StoredDiagnosis = {
        id,
        target: status.target,
        events: status.events || [],
        lastSeqNum: status.events?.length
          ? Math.max(...status.events.map((e) => e.seq_num))
          : 0,
        status: status.status as 'running' | 'completed' | 'failed',
        result: status.result,
        cleanup: null,
      };
      this.diagnoses.set(id, stored);

      if (stored.status === 'running') {
        this.connectSSE(id, stored.lastSeqNum, onUpdate);
      }
    } catch {
      // Server restart or unknown ID — will be created fresh
    }
  }

  private connectSSE(id: string, since: number, onUpdate: () => void): void {
    const stored = this.diagnoses.get(id);
    if (!stored) return;

    stored.cleanup?.();

    stored.cleanup = subscribeDiagnosis(id, since, {
      onEvent: (ev) => {
        stored.events.push(ev);
        stored.lastSeqNum = Math.max(stored.lastSeqNum, ev.seq_num);
        onUpdate();
      },
      onComplete: (status) => {
        if (status === 'completed' || status === 'failed' || status === 'running') {
          stored.status = status;
        }
        stored.cleanup = null;
        onUpdate();
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