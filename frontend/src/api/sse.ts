import { api } from './client';
import type { DiagnosisEvent, DiagnosisStatus } from './types';

export interface SSECallbacks {
  onEvent: (ev: DiagnosisEvent) => void;
  onComplete: (status: string) => void;
  onError?: (err: string) => void;
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
    try {
      const data = JSON.parse(e.data);
      callbacks.onComplete(data.status);
    } catch { /* ignore */ }
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
