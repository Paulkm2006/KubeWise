import { AUTH_HEADER } from '../config';
import type { ChatSSEEvent } from './types';

export interface ChatSSECallbacks {
  onEvent: (ev: ChatSSEEvent) => void;
  onComplete: () => void;
  onError?: (err: string) => void;
}

export function subscribeChat(streamUrl: string, callbacks: ChatSSECallbacks): () => void {
  const controller = new AbortController();

  (async () => {
    try {
      const headers: Record<string, string> = {};
      if (AUTH_HEADER) headers['Authorization'] = AUTH_HEADER;
      const res = await fetch(streamUrl, {
        headers,
        signal: controller.signal,
      });
      if (!res.ok) {
        callbacks.onError?.(`SSE ${res.status}`);
        return;
      }
      const reader = res.body?.getReader();
      if (!reader) {
        callbacks.onError?.('No response body');
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
          for (const line of part.split('\n')) {
            if (line.startsWith('event:')) eventType = line.slice(6).trim();
            else if (line.startsWith('data:')) data += (data ? '\n' : '') + line.slice(5).trim();
          }
          if (!eventType) continue;
          try {
            const parsed = data ? (JSON.parse(data) as Record<string, unknown>) : {};
            const event = { type: eventType as ChatSSEEvent['type'], ...parsed } as ChatSSEEvent;
            if (eventType === 'stream_done') {
              callbacks.onComplete();
              return;
            }
            if (eventType === 'stream_err') {
              callbacks.onEvent(event);
              callbacks.onError?.(String(parsed.error || 'stream error'));
              return;
            }
            callbacks.onEvent(event);
          } catch { /* skip malformed */ }
        }
      }
    } catch (err) {
      if (!controller.signal.aborted) {
        callbacks.onError?.(err instanceof Error ? err.message : 'Connection failed');
      }
    }
  })();

  return () => controller.abort();
}
