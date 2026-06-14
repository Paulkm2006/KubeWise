const raw = import.meta.env.VITE_API_BASE?.trim();

/** API base URL, e.g. `/api/v1` (Docker/nginx) or `http://localhost:3000/api/v1` (local dev). */
export const API_BASE = raw && raw.length > 0 ? raw.replace(/\/$/, '') : '/api/v1';

const apiUser = import.meta.env.VITE_API_USER?.trim();
const apiPass = import.meta.env.VITE_API_PASS?.trim();

/** Basic auth header value, or empty string if credentials are not configured. */
export const AUTH_HEADER = apiUser && apiPass ? 'Basic ' + btoa(`${apiUser}:${apiPass}`) : '';
