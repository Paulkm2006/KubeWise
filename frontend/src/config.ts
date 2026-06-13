const raw = import.meta.env.VITE_API_BASE?.trim();

/** API base URL, e.g. `/api/v1` (Docker/nginx) or `http://localhost:3000/api/v1` (local dev). */
export const API_BASE = raw && raw.length > 0 ? raw.replace(/\/$/, '') : '/api/v1';
