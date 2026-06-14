import { api } from './client';
import type { AuditEvent, AuditStatus, DiagnosisEvent, DiagnosisStatus } from './types';

export interface SSECallbacks {
  onEvent: (ev: DiagnosisEvent) => void;
  onComplete: (status: string) => void;
  onError?: (err: string) => void;
}

function parseTerminalStatus(data: string): string {
  try {
    const payload = JSON.parse(data) as { status?: unknown };
    if (typeof payload.status === 'string' && payload.status.length > 0) {
      return payload.status;
    }
  } catch {
    /* ignore malformed terminal payload */
  }
  return 'completed';
}

export function subscribeDiagnosis(
  id: string,
  since: number,
  callbacks: SSECallbacks
): () => void {
  const url = api.diagnoses.eventsUrl(id, since);
  const es = new EventSource(url);

  es.addEventListener('diagnosis_event', (e: MessageEvent) => {
    try {
      const event: DiagnosisEvent = {
        ...JSON.parse(e.data),
        seq_num: parseInt(e.lastEventId, 10) || 0,
      };
      callbacks.onEvent(event);
    } catch { /* skip malformed */ }
  });

  es.addEventListener('stream_complete', (e: MessageEvent) => {
    callbacks.onComplete(parseTerminalStatus(e.data));
    es.close();
  });

  es.addEventListener('stream_err', (e: MessageEvent) => {
    try {
      const data = JSON.parse(e.data);
      callbacks.onError?.(data.error || 'stream error');
    } catch { /* ignore */ }
    es.close();
  });

  es.onerror = () => {
    if (es.readyState === 2) {
      callbacks.onError?.('Connection failed');
    }
  };

  return () => es.close();
}

export async function fetchDiagnosisStatus(id: string): Promise<DiagnosisStatus> {
  return api.diagnoses.get(id);
}

export interface AuditSSECallbacks {
  onEvent: (ev: AuditEvent) => void;
  onComplete: (status: string) => void;
  onError?: (err: string) => void;
}

export function subscribeAudit(
  id: string,
  since: number,
  callbacks: AuditSSECallbacks,
): () => void {
  const url = api.audits.eventsUrl(id, since);
  const es = new EventSource(url);

  es.addEventListener('audit_event', (e: MessageEvent) => {
    try {
      const event: AuditEvent = {
        ...JSON.parse(e.data),
        seq_num: parseInt(e.lastEventId, 10) || 0,
      };
      callbacks.onEvent(event);
    } catch { /* skip malformed */ }
  });

  es.addEventListener('stream_complete', (e: MessageEvent) => {
    callbacks.onComplete(parseTerminalStatus(e.data));
    es.close();
  });

  es.addEventListener('stream_err', (e: MessageEvent) => {
    try {
      const data = JSON.parse(e.data);
      callbacks.onError?.(data.error || 'stream error');
    } catch { /* ignore */ }
    es.close();
  });

  es.onerror = () => {
    if (es.readyState === 2) {
      callbacks.onError?.('Connection failed');
    }
  };

  return () => es.close();
}

export async function fetchAuditStatus(id: string): Promise<AuditStatus> {
  return api.audits.get(id);
}
