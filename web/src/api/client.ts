import type {
  Target,
  CreateTargetRequest,
  UpdateTargetRequest,
  Scan,
  Agent,
  CreateAgentResponse,
  AgentDownloads,
} from './types';

// DC API base URL is resolved at call time from the active tenant context.
// In prod, each tenant lives on a specific DC (possibly different from the
// one the frontend was deployed alongside); using the active DC URL means
// the tenant frontend works for tenants in any region.
//
// For local dev (or if no DC URL is set yet), fall back to '' which means
// same-origin (nginx proxy).
const TOKEN_KEY = 'silkstrand_token';
const DC_URL_KEY = 'silkstrand_dc_api_url';

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY);
}

export function setToken(token: string): void {
  localStorage.setItem(TOKEN_KEY, token);
}

export function clearToken(): void {
  localStorage.removeItem(TOKEN_KEY);
  localStorage.removeItem(DC_URL_KEY);
}

export function setDCApiURL(url: string | null | undefined): void {
  if (url) localStorage.setItem(DC_URL_KEY, url);
  else localStorage.removeItem(DC_URL_KEY);
}

function dcBaseURL(): string {
  return localStorage.getItem(DC_URL_KEY) || import.meta.env.VITE_API_URL || '';
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

  const res = await fetch(`${dcBaseURL()}${path}`, { ...init, headers });

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

// Agents
export const listAgents = () => request<Agent[]>('/api/v1/agents');
export const createAgent = (name: string, version?: string) =>
  request<CreateAgentResponse>('/api/v1/agents', {
    method: 'POST',
    body: JSON.stringify({ name, version }),
  });
export const rotateAgentKey = (id: string) =>
  request<{ api_key: string }>(`/api/v1/agents/${id}/rotate-key`, { method: 'POST' });
export const deleteAgent = (id: string) =>
  request<void>(`/api/v1/agents/${id}`, { method: 'DELETE' });
export const getAgentDownloads = () =>
  request<AgentDownloads>('/api/v1/agents/downloads');
