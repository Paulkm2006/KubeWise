import type { ChatSSEEvent } from './types';

export interface ChatSSECallbacks {
  onEvent: (ev: ChatSSEEvent) => void;
  onComplete: () => void;
  onError?: (err: string) => void;
}

export function subscribeChat(streamUrl: string, callbacks: ChatSSECallbacks): () => void {
  const es = new EventSource(streamUrl);

  const bind = (type: ChatSSEEvent['type'], handler: (data: Record<string, unknown>) => void) => {
    es.addEventListener(type, (e: MessageEvent) => {
      try {
        const data = JSON.parse(e.data) as Record<string, unknown>;
        handler(data);
      } catch {
        /* skip malformed */
      }
    });
  };

  bind('agent_start', (d) => callbacks.onEvent({ type: 'agent_start', ...d }));
  bind('agent_done', (d) => callbacks.onEvent({ type: 'agent_done', ...d }));
  bind('phase', (d) => callbacks.onEvent({ type: 'phase', ...d }));
  bind('tool_call', (d) => callbacks.onEvent({ type: 'tool_call', ...d }));
  bind('tool_done', (d) => callbacks.onEvent({ type: 'tool_done', ...d }));
  bind('tool_fail', (d) => callbacks.onEvent({ type: 'tool_fail', ...d }));
  bind('llm_text_delta', (d) => callbacks.onEvent({ type: 'llm_text_delta', ...d }));
  bind('supervisor', (d) => callbacks.onEvent({ type: 'supervisor', ...d }));
  bind('interaction_request', (d) => callbacks.onEvent({ type: 'interaction_request', ...d }));
  bind('stream_done', () => {
    callbacks.onComplete();
    es.close();
  });
  bind('stream_err', (d) => {
    callbacks.onEvent({ type: 'stream_err', error: String(d.error || 'stream error') });
    callbacks.onError?.(String(d.error || 'stream error'));
    es.close();
  });

  es.onerror = () => {
    if (es.readyState === EventSource.CLOSED) {
      callbacks.onError?.('Connection closed');
    }
  };

  return () => es.close();
}
