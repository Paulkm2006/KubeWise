import { api } from './client';
import { AUTH_HEADER } from '../config';
import type { AuditEvent, AuditStatus, DiagnosisEvent, DiagnosisStatus } from './types';

export interface SSECallbacks {
  onEvent: (ev: DiagnosisEvent) => void;
  onComplete: (status: string) => void;
  onError?: (err: string) => void;
}

function parseSSELines(text: string): { event?: string; data?: string; id?: string }[] {
  const blocks = text.split('\n\n');
  return blocks.map((block) => {
    const result: { event?: string; data?: string; id?: string } = {};
    for (const line of block.split('\n')) {
      if (line.startsWith('event:')) result.event = line.slice(6).trim();
      else if (line.startsWith('data:')) result.data = (result.data ? result.data + '\n' : '') + line.slice(5).trim();
      else if (line.startsWith('id:')) result.id = line.slice(3).trim();
    }
    return result;
  });
}

function subscribeSSE(
  url: string,
  handlers: { eventTypes: string[]; onEvent: (type: string, data: string, id: string) => void; onComplete: () => void; onError: (err: string) => void },
): () => void {
  const controller = new AbortController();

  (async () => {
    try {
      const headers: Record<string, string> = {};
      if (AUTH_HEADER) headers['Authorization'] = AUTH_HEADER;
      const res = await fetch(url, {
        headers,
        signal: controller.signal,
      });
      if (!res.ok) {
        handlers.onError(`SSE ${res.status}`);
        return;
      }
      const reader = res.body?.getReader();
      if (!reader) {
        handlers.onError('No response body');
        return;
      }
      const decoder = new TextDecoder();
      let buffer = '';
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        const parts = buffer.split('\n\n');
        buffer = parts.pop() || '';
        for (const part of parts) {
          let eventType = '';
          let data = '';
          let id = '';
          for (const line of part.split('\n')) {
            if (line.startsWith('event:')) eventType = line.slice(6).trim();
            else if (line.startsWith('data:')) data += (data ? '\n' : '') + line.slice(5).trim();
            else if (line.startsWith('id:')) id = line.slice(3).trim();
          }
          if (eventType) {
            if (eventType === 'stream_complete') {
              handlers.onComplete();
              return;
            }
            if (eventType === 'stream_err') {
              try { handlers.onError(JSON.parse(data).error || 'stream error'); } catch { handlers.onError('stream error'); }
              return;
            }
            handlers.onEvent(eventType, data, id);
          }
        }
      }
    } catch (err) {
      if (!controller.signal.aborted) {
        handlers.onError(err instanceof Error ? err.message : 'Connection failed');
      }
    }
  })();

  return () => controller.abort();
}

export function subscribeDiagnosis(
  id: string,
  since: number,
  callbacks: SSECallbacks
): () => void {
  const url = api.diagnoses.eventsUrl(id, since);
  return subscribeSSE(url, {
    eventTypes: ['diagnosis_event', 'stream_complete', 'stream_err'],
    onEvent: (type, data, id) => {
      if (type === 'diagnosis_event') {
        try {
          const event: DiagnosisEvent = {
            ...JSON.parse(data),
            seq_num: parseInt(id, 10) || 0,
          };
          callbacks.onEvent(event);
        } catch { /* skip malformed */ }
      }
    },
    onComplete: () => callbacks.onComplete('completed'),
    onError: (err) => callbacks.onError?.(err),
  });
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
  return subscribeSSE(url, {
    eventTypes: ['audit_event', 'stream_complete', 'stream_err'],
    onEvent: (type, data, id) => {
      if (type === 'audit_event') {
        try {
          const event: AuditEvent = {
            ...JSON.parse(data),
            seq_num: parseInt(id, 10) || 0,
          };
          callbacks.onEvent(event);
        } catch { /* skip malformed */ }
      }
    },
    onComplete: () => callbacks.onComplete('completed'),
    onError: (err) => callbacks.onError?.(err),
  });
}

export async function fetchAuditStatus(id: string): Promise<AuditStatus> {
  return api.audits.get(id);
}
