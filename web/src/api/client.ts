import type {
  Target,
  CreateTargetRequest,
  UpdateTargetRequest,
  Scan,
} from './types';

const BASE_URL = import.meta.env.VITE_API_URL || '';
const TOKEN_KEY = 'silkstrand_dev_token';

// Module-level token getter that can be overridden by Clerk auth
let clerkTokenGetter: (() => Promise<string | null>) | null = null;

/** Register a Clerk token getter function. Called by ClerkAuthProvider. */
export function setClerkTokenGetter(getter: () => Promise<string | null>): void {
  clerkTokenGetter = getter;
}

/** Clear the Clerk token getter (e.g. on sign-out). */
export function clearClerkTokenGetter(): void {
  clerkTokenGetter = null;
}

async function getToken(): Promise<string | null> {
  // Prefer Clerk token if a getter is registered
  if (clerkTokenGetter) {
    const token = await clerkTokenGetter();
    if (token) return token;
  }
  // Fall back to dev token from localStorage
  return localStorage.getItem(TOKEN_KEY);
}

export function hasDevToken(): boolean {
  return localStorage.getItem(TOKEN_KEY) !== null;
}

// Keep hasToken as alias for backward compatibility with dev flow
export function hasToken(): boolean {
  return hasDevToken();
}

export function setToken(token: string): void {
  localStorage.setItem(TOKEN_KEY, token);
}

export function clearToken(): void {
  localStorage.removeItem(TOKEN_KEY);
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const token = await getToken();
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(init?.headers as Record<string, string> | undefined),
  };
  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }

  const res = await fetch(`${BASE_URL}${path}`, {
    ...init,
    headers,
  });

  if (!res.ok) {
    let message = `${res.status} ${res.statusText}`;
    try {
      const body = await res.json();
      if (body.error) {
        message = body.error;
      }
    } catch {
      // ignore parse errors
    }
    throw new Error(message);
  }

  if (res.status === 204) {
    return undefined as T;
  }

  return res.json() as Promise<T>;
}

// Targets

export function listTargets(): Promise<Target[]> {
  return request<Target[]>('/api/v1/targets');
}

export function createTarget(req: CreateTargetRequest): Promise<Target> {
  return request<Target>('/api/v1/targets', {
    method: 'POST',
    body: JSON.stringify(req),
  });
}

export function getTarget(id: string): Promise<Target> {
  return request<Target>(`/api/v1/targets/${id}`);
}

export function updateTarget(id: string, req: UpdateTargetRequest): Promise<Target> {
  return request<Target>(`/api/v1/targets/${id}`, {
    method: 'PUT',
    body: JSON.stringify(req),
  });
}

export function deleteTarget(id: string): Promise<void> {
  return request<void>(`/api/v1/targets/${id}`, {
    method: 'DELETE',
  });
}

// Scans

export function listScans(): Promise<Scan[]> {
  return request<Scan[]>('/api/v1/scans');
}

export function createScan(targetId: string, bundleId: string): Promise<Scan> {
  return request<Scan>('/api/v1/scans', {
    method: 'POST',
    body: JSON.stringify({ target_id: targetId, bundle_id: bundleId }),
  });
}

export function getScan(id: string): Promise<Scan> {
  return request<Scan>(`/api/v1/scans/${id}`);
}
