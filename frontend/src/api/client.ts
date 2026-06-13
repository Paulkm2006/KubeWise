import type {
  ClusterSummary,
  Issue,
  DiagnosisSummary,
  DiagnosisStatus,
  Activity,
  DiagnosisListResponse,
  AuditStatus,
  AuditListResponse,
} from './types';
import { API_BASE, AUTH_HEADER } from '../config';

async function fetchJSON<T>(url: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers);
  if (AUTH_HEADER) headers.set('Authorization', AUTH_HEADER);
  const res = await fetch(url, { ...init, headers });
  if (!res.ok) {
    const body = await res.text();
    throw new Error(`API ${res.status}: ${body}`);
  }
  return res.json();
}

export const api = {
  clusters: {
    list: () => fetchJSON<ClusterSummary[]>(`${API_BASE}/clusters`),
    issues: (name: string) => fetchJSON<Issue[]>(`${API_BASE}/clusters/${encodeURIComponent(name)}/issues`),
    events: (name: string) => fetchJSON<unknown[]>(`${API_BASE}/clusters/${encodeURIComponent(name)}/events`),
  },
  chats: {
    sync: (body: { query: string; query_id?: string; session_id?: string; cluster?: string }) =>
      fetchJSON<{ query_id: string; result: string }>(`${API_BASE}/chats`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      }),
    streamUrl: (params: { query: string; query_id?: string; session_id?: string; cluster?: string }) => {
      const q = new URLSearchParams({ query: params.query });
      if (params.query_id) q.set('query_id', params.query_id);
      if (params.session_id) q.set('session_id', params.session_id);
      if (params.cluster) q.set('cluster', params.cluster);
      return `${API_BASE}/chats/stream?${q}`;
    },
    answerInteraction: (body: { interaction_id: string; payload: unknown }) =>
      fetchJSON<{ status: string }>(`${API_BASE}/chats/interactions`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      }),
  },
  sessions: {
    list: () => fetchJSON<{ sessions: unknown[] }>(`${API_BASE}/sessions`),
    create: (title?: string) =>
      fetchJSON<unknown>(`${API_BASE}/sessions`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ title: title ?? '' }),
      }),
    get: (id: string) => fetchJSON<unknown>(`${API_BASE}/sessions/${encodeURIComponent(id)}`),
    delete: (id: string) =>
      fetchJSON<{ status: string }>(`${API_BASE}/sessions/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  },
  diagnoses: {
    list: async (params?: { limit?: number; offset?: number }) => {
      const q = new URLSearchParams();
      if (params?.limit) q.set('limit', String(params.limit));
      if (params?.offset) q.set('offset', String(params.offset));
      const res = await fetchJSON<DiagnosisListResponse>(`${API_BASE}/diagnoses?${q}`);
      return res.diagnoses;
    },
    get: (id: string) => fetchJSON<DiagnosisStatus>(`${API_BASE}/diagnoses/${encodeURIComponent(id)}`),
    latest: (params: { cluster: string; namespace: string; pod: string }) => {
      const q = new URLSearchParams(params);
      return fetchJSON<DiagnosisStatus>(`${API_BASE}/diagnoses/latest?${q}`);
    },
    cancel: (id: string) =>
      fetchJSON<{ status: string; diagnosis_id: string }>(
        `${API_BASE}/diagnoses/${encodeURIComponent(id)}/cancel`,
        { method: 'POST' },
      ),
    create: (cluster: string, namespace: string, pod: string) =>
      fetchJSON<{ diagnosis_id: string; status: string }>(`${API_BASE}/diagnoses`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ cluster, namespace, pod }),
      }),
    eventsUrl: (id: string, since = 0) =>
      `${API_BASE}/diagnoses/${encodeURIComponent(id)}/events?since=${since}`,
  },
  activities: {
    list: (limit = 20, offset = 0) =>
      fetchJSON<Activity[]>(`${API_BASE}/activities?limit=${limit}&offset=${offset}`),
  },
  audits: {
    list: async (params?: { limit?: number; offset?: number }) => {
      const q = new URLSearchParams();
      if (params?.limit) q.set('limit', String(params.limit));
      if (params?.offset) q.set('offset', String(params.offset));
      const res = await fetchJSON<AuditListResponse>(`${API_BASE}/audits?${q}`);
      return res.audits;
    },
    get: (id: string) => fetchJSON<AuditStatus>(`${API_BASE}/audits/${encodeURIComponent(id)}`),
    latest: (cluster: string) =>
      fetchJSON<AuditStatus>(`${API_BASE}/audits/latest?cluster=${encodeURIComponent(cluster)}`),
    cancel: (id: string) =>
      fetchJSON<{ status: string; audit_id: string }>(
        `${API_BASE}/audits/${encodeURIComponent(id)}/cancel`,
        { method: 'POST' },
      ),
    create: (cluster: string) =>
      fetchJSON<{ audit_id: string; status: string }>(`${API_BASE}/audits`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ cluster }),
      }),
    eventsUrl: (id: string, since = 0) =>
      `${API_BASE}/audits/${encodeURIComponent(id)}/events?since=${since}`,
  },
};
