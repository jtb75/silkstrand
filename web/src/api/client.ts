import type {
  Target,
  CreateTargetRequest,
  UpdateTargetRequest,
  Scan,
} from './types';

const BASE_URL = import.meta.env.VITE_API_URL || '';
const TOKEN_KEY = 'silkstrand_token';

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY);
}

export function setToken(token: string): void {
  localStorage.setItem(TOKEN_KEY, token);
}

export function clearToken(): void {
  localStorage.removeItem(TOKEN_KEY);
}

export function hasToken(): boolean {
  return getToken() !== null;
}

// --- helpers kept for backwards compat with tests / legacy callers ---
export function hasDevToken(): boolean { return hasToken(); }

/**
 * request sends a JSON HTTP request to the DC API. The JWT in localStorage
 * is attached as a bearer token. On 401, the token is cleared and the
 * caller sees an error — callers render the login page from the auth
 * context, not via redirects here.
 */
async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const token = getToken();
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(init?.headers as Record<string, string> | undefined),
  };
  if (token) headers['Authorization'] = `Bearer ${token}`;

  const res = await fetch(`${BASE_URL}${path}`, { ...init, headers });

  if (res.status === 401) {
    clearToken();
    throw new Error('Unauthorized');
  }

  if (!res.ok) {
    let message = `${res.status} ${res.statusText}`;
    try {
      const body = await res.json();
      if (body.error) message = body.error;
    } catch {
      /* empty */
    }
    throw new Error(message);
  }

  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

// Targets
export const listTargets = () => request<Target[]>('/api/v1/targets');
export const createTarget = (req: CreateTargetRequest) =>
  request<Target>('/api/v1/targets', { method: 'POST', body: JSON.stringify(req) });
export const getTarget = (id: string) => request<Target>(`/api/v1/targets/${id}`);
export const updateTarget = (id: string, req: UpdateTargetRequest) =>
  request<Target>(`/api/v1/targets/${id}`, { method: 'PUT', body: JSON.stringify(req) });
export const deleteTarget = (id: string) =>
  request<void>(`/api/v1/targets/${id}`, { method: 'DELETE' });

// Scans
export const listScans = () => request<Scan[]>('/api/v1/scans');
export const createScan = (targetId: string, bundleId: string) =>
  request<Scan>('/api/v1/scans', {
    method: 'POST',
    body: JSON.stringify({ target_id: targetId, bundle_id: bundleId }),
  });
export const getScan = (id: string) => request<Scan>(`/api/v1/scans/${id}`);
