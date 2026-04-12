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
  status: 'active' | 'suspended';
  created_at: string;
}

export interface PendingInvite {
  id: string;
  email: string;
  role: 'admin' | 'member';
  expires_at: string;
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

  previewInvitation: (token: string) =>
    get<{
      email: string;
      role: 'admin' | 'member';
      tenant_name: string;
      existing_user: boolean;
    }>(`/api/v1/tenant-auth/invitation-preview?token=${encodeURIComponent(token)}`),

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

  updateMemberStatus: (jwt: string, userId: string, status: 'active' | 'suspended') =>
    fetch(`${BASE_URL}/api/v1/tenant-auth/members/${userId}/status`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${jwt}` },
      body: JSON.stringify({ status }),
    }).then((res) => handle<void>(res)),

  listInvitations: (jwt: string) =>
    get<PendingInvite[]>('/api/v1/tenant-auth/invitations', jwt),

  cancelInvitation: (jwt: string, id: string) =>
    del(`/api/v1/tenant-auth/invitations/${id}`, jwt),

  resendInvitation: (jwt: string, id: string) =>
    post<void>(`/api/v1/tenant-auth/invitations/${id}/resend`, {}, jwt),

  updateMemberRole: (jwt: string, userId: string, role: 'admin' | 'member') =>
    fetch(`${BASE_URL}/api/v1/tenant-auth/members/${userId}/role`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${jwt}` },
      body: JSON.stringify({ role }),
    }).then((res) => handle<void>(res)),

  updateProfile: (jwt: string, displayName: string) =>
    fetch(`${BASE_URL}/api/v1/tenant-auth/me`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${jwt}` },
      body: JSON.stringify({ display_name: displayName }),
    }).then((res) => handle<void>(res)),

  changePassword: (jwt: string, currentPassword: string, newPassword: string) =>
    fetch(`${BASE_URL}/api/v1/tenant-auth/me/password`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${jwt}` },
      body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
    }).then((res) => handle<void>(res)),
};
