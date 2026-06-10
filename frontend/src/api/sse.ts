import type { StreamEvent } from './types';

const BASE = 'http://localhost:3000/api/v1';

export function subscribeDiagnosis(
  id: string,
  onEvent: (ev: StreamEvent) => void,
  onDone: () => void
): () => void {
  const es = new EventSource(`${BASE}/diagnose/stream?id=${encodeURIComponent(id)}`);

  es.addEventListener('diagnosis_event', (e: MessageEvent) => {
    try {
      const data = JSON.parse(e.data) as StreamEvent;
      onEvent(data);
    } catch { /* skip malformed */ }
  });

  es.addEventListener('stream_done', () => {
    es.close();
    onDone();
  });

  es.addEventListener('stream_err', (e: MessageEvent) => {
    try {
      const data = JSON.parse(e.data);
      console.error('Diagnosis SSE error:', data.error);
    } catch { /* ignore */ }
    es.close();
  });

  es.onerror = () => { es.close(); };
  return () => es.close();
}
