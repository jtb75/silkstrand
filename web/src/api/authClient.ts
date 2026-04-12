// Client for the backoffice's tenant-auth endpoints. These calls go to
// the backoffice (not the DC API), proxied by nginx under /backoffice-api/.
import type { User, Membership, ActiveTenant } from './types';

const BASE_URL = import.meta.env.VITE_BACKOFFICE_URL || '';

export interface LoginResponse {
  token: string;
  user: { id: string; email: string };
  active: ActiveTenant;
}

export interface MeResponse {
  user: User;
  memberships: Membership[];
  active: ActiveTenant;
}

async function post<T>(path: string, body: unknown, token?: string | null): Promise<T> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' };
  if (token) headers['Authorization'] = `Bearer ${token}`;
  const res = await fetch(`${BASE_URL}${path}`, {
    method: 'POST',
    headers,
    body: JSON.stringify(body),
  });
  return handle<T>(res);
}

async function get<T>(path: string, token?: string | null): Promise<T> {
  const headers: Record<string, string> = {};
  if (token) headers['Authorization'] = `Bearer ${token}`;
  const res = await fetch(`${BASE_URL}${path}`, { headers });
  return handle<T>(res);
}

async function handle<T>(res: Response): Promise<T> {
  if (res.status === 204) return undefined as T;
  if (!res.ok) {
    let msg = `${res.status} ${res.statusText}`;
    try {
      const body = await res.json();
      if (body.error) msg = body.error;
    } catch { /* empty */ }
    throw new Error(msg);
  }
  return res.json() as Promise<T>;
}

export interface TenantMember {
  user_id: string;
  email: string;
  role: 'admin' | 'member';
  created_at: string;
}

async function del(path: string, token: string): Promise<void> {
  const res = await fetch(`${BASE_URL}${path}`, {
    method: 'DELETE',
    headers: { Authorization: `Bearer ${token}` },
  });
  await handle<void>(res);
}

export const authApi = {
  login: (email: string, password: string) =>
    post<LoginResponse>('/api/v1/tenant-auth/login', { email, password }),

  acceptInvite: (token: string, password: string) =>
    post<LoginResponse>('/api/v1/tenant-auth/accept-invite', { token, password }),

  forgotPassword: (email: string) =>
    post<void>('/api/v1/tenant-auth/forgot-password', { email }),

  resetPassword: (token: string, password: string) =>
    post<void>('/api/v1/tenant-auth/reset-password', { token, password }),

  me: (jwt: string) =>
    get<MeResponse>('/api/v1/tenant-auth/me', jwt),

  switchOrg: (jwt: string, tenantId: string) =>
    post<LoginResponse>('/api/v1/tenant-auth/switch-org', { tenant_id: tenantId }, jwt),

  listMembers: (jwt: string) =>
    get<TenantMember[]>('/api/v1/tenant-auth/members', jwt),

  invite: (jwt: string, email: string, role: 'admin' | 'member') =>
    post<{ status: string }>('/api/v1/tenant-auth/invites', { email, role }, jwt),

  removeMember: (jwt: string, userId: string) =>
    del(`/api/v1/tenant-auth/members/${userId}`, jwt),
};
