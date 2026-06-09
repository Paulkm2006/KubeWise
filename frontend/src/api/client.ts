import type { ClusterSummary, Issue, DiagnosisSummary, DiagnosisDetail, Activity } from './types';

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
  },
  diagnoses: {
    list: (params?: { cluster?: string; limit?: number }) => {
      const q = new URLSearchParams();
      if (params?.cluster) q.set('cluster', params.cluster);
      if (params?.limit) q.set('limit', String(params.limit));
      return fetchJSON<DiagnosisSummary[]>(`${BASE}/diagnoses?${q}`);
    },
    get: (id: string) => fetchJSON<DiagnosisDetail>(`${BASE}/diagnoses/${encodeURIComponent(id)}`),
    create: (cluster: string, namespace: string, pod: string) =>
      fetchJSON<{ diagnosis_id: string; status: string }>(`${BASE}/diagnose`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ cluster, namespace, pod }),
      }),
  },
  activities: {
    list: (limit = 20) => fetchJSON<Activity[]>(`${BASE}/activities?limit=${limit}`),
  },
};
