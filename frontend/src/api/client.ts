import type {
  ClusterSummary,
  Issue,
  DiagnosisSummary,
  DiagnosisStatus,
  Activity,
  DiagnosisListResponse,
} from './types';

const BASE = 'http://localhost:3000/api/v1';

async function fetchJSON<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, init);
  if (!res.ok) {
    const body = await res.text();
    throw new Error(`API ${res.status}: ${body}`);
  }
  return res.json();
}

export const api = {
  clusters: {
    list: () => fetchJSON<ClusterSummary[]>(`${BASE}/clusters`),
    issues: (name: string) => fetchJSON<Issue[]>(`${BASE}/clusters/${encodeURIComponent(name)}/issues`),
    events: (name: string) => fetchJSON<unknown[]>(`${BASE}/clusters/${encodeURIComponent(name)}/events`),
  },
  chats: {
    sync: (body: { query: string; query_id?: string; session_id?: string; cluster?: string }) =>
      fetchJSON<{ query_id: string; result: string }>(`${BASE}/chats`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      }),
    streamUrl: (params: { query: string; query_id?: string; session_id?: string; cluster?: string }) => {
      const q = new URLSearchParams({ query: params.query });
      if (params.query_id) q.set('query_id', params.query_id);
      if (params.session_id) q.set('session_id', params.session_id);
      if (params.cluster) q.set('cluster', params.cluster);
      return `${BASE}/chats/stream?${q}`;
    },
    answerInteraction: (body: { interaction_id: string; payload: unknown }) =>
      fetchJSON<{ status: string }>(`${BASE}/chats/interactions`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      }),
  },
  sessions: {
    list: () => fetchJSON<{ sessions: unknown[] }>(`${BASE}/sessions`),
    create: (title?: string) =>
      fetchJSON<unknown>(`${BASE}/sessions`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ title: title ?? '' }),
      }),
    get: (id: string) => fetchJSON<unknown>(`${BASE}/sessions/${encodeURIComponent(id)}`),
    delete: (id: string) =>
      fetchJSON<{ status: string }>(`${BASE}/sessions/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  },
  diagnoses: {
    list: async (params?: { limit?: number; offset?: number }) => {
      const q = new URLSearchParams();
      if (params?.limit) q.set('limit', String(params.limit));
      if (params?.offset) q.set('offset', String(params.offset));
      const res = await fetchJSON<DiagnosisListResponse>(`${BASE}/diagnoses?${q}`);
      return res.diagnoses;
    },
    get: (id: string) => fetchJSON<DiagnosisStatus>(`${BASE}/diagnoses/${encodeURIComponent(id)}`),
    latest: (params: { cluster: string; namespace: string; pod: string }) => {
      const q = new URLSearchParams(params);
      return fetchJSON<DiagnosisStatus>(`${BASE}/diagnoses/latest?${q}`);
    },
    cancel: (id: string) =>
      fetchJSON<{ status: string; diagnosis_id: string }>(
        `${BASE}/diagnoses/${encodeURIComponent(id)}/cancel`,
        { method: 'POST' },
      ),
    create: (cluster: string, namespace: string, pod: string) =>
      fetchJSON<{ diagnosis_id: string; status: string }>(`${BASE}/diagnoses`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ cluster, namespace, pod }),
      }),
    eventsUrl: (id: string, since = 0) =>
      `${BASE}/diagnoses/${encodeURIComponent(id)}/events?since=${since}`,
  },
  activities: {
    list: (limit = 20, offset = 0) =>
      fetchJSON<Activity[]>(`${BASE}/activities?limit=${limit}&offset=${offset}`),
  },
};
