import type {
  Target,
  CreateTargetRequest,
  UpdateTargetRequest,
  Scan,
  Agent,
  CreateAgentResponse,
  AgentDownloads,
  Bundle,
  AssetListResponse,
  AssetDetailResponse,
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
export const deleteScan = (id: string) =>
  request<void>(`/api/v1/scans/${id}`, { method: 'DELETE' });
export const listBundles = () => request<Bundle[]>('/api/v1/bundles');

// Assets (ADR 003 R1a)
export interface AssetFilterParams {
  service?: string;
  service_in?: string[];
  ip?: string;
  source?: 'manual' | 'discovered';
  compliance_status?: string;
  cve_count_gte?: number;
  new_since?: string;     // e.g. "7d"
  changed_since?: string; // e.g. "7d"
  q?: string;
  sort_by?: 'last_seen' | 'first_seen' | 'ip' | 'service' | 'cve_count';
  sort_dir?: 'asc' | 'desc';
  page?: number;
  page_size?: number;
}

function buildAssetQuery(params: AssetFilterParams): string {
  const u = new URLSearchParams();
  if (params.service) u.set('service', params.service);
  if (params.service_in?.length) u.set('service_in', params.service_in.join(','));
  if (params.ip) u.set('ip', params.ip);
  if (params.source) u.set('source', params.source);
  if (params.compliance_status) u.set('compliance_status', params.compliance_status);
  if (params.cve_count_gte != null) u.set('cve_count_gte', String(params.cve_count_gte));
  if (params.new_since) u.set('new_since', params.new_since);
  if (params.changed_since) u.set('changed_since', params.changed_since);
  if (params.q) u.set('q', params.q);
  if (params.sort_by) u.set('sort_by', params.sort_by);
  if (params.sort_dir) u.set('sort_dir', params.sort_dir);
  if (params.page) u.set('page', String(params.page));
  if (params.page_size) u.set('page_size', String(params.page_size));
  return u.toString();
}

export const listAssets = (params: AssetFilterParams = {}) =>
  request<AssetListResponse>(`/api/v1/assets?${buildAssetQuery(params)}`);

export const getAsset = (id: string, eventsLimit = 50) =>
  request<AssetDetailResponse>(`/api/v1/assets/${id}?events=${eventsLimit}`);

export const promoteAsset = (id: string, bundleId: string) =>
  request<{ target: Target; bundle_id: string }>(`/api/v1/assets/${id}/promote`, {
    method: 'POST',
    body: JSON.stringify({ bundle_id: bundleId }),
  });

// Correlation rules (ADR 003 R1b)
import type { CorrelationRule, CorrelationRuleBody } from './types';
export const listCorrelationRules = () =>
  request<CorrelationRule[]>('/api/v1/correlation-rules');

export interface UpsertRuleRequest {
  name: string;
  trigger: 'asset_discovered' | 'asset_event';
  enabled?: boolean;
  event_type_filter?: string;
  body: CorrelationRuleBody;
}

export const createCorrelationRule = (req: UpsertRuleRequest) =>
  request<CorrelationRule>('/api/v1/correlation-rules', {
    method: 'POST',
    body: JSON.stringify(req),
  });

export const updateCorrelationRule = (id: string, req: UpsertRuleRequest) =>
  request<CorrelationRule>(`/api/v1/correlation-rules/${id}`, {
    method: 'PUT',
    body: JSON.stringify(req),
  });

export const deleteCorrelationRule = (id: string) =>
  request<void>(`/api/v1/correlation-rules/${id}`, { method: 'DELETE' });

// Notification channels (ADR 003 R1c-a / D12)
import type { NotificationChannel } from './types';
export const listNotificationChannels = () =>
  request<NotificationChannel[]>('/api/v1/notification-channels');

export interface UpsertChannelRequest {
  name: string;
  type: 'webhook' | 'slack' | 'email' | 'pagerduty';
  enabled?: boolean;
  config: Record<string, unknown>;
}

export const createNotificationChannel = (req: UpsertChannelRequest) =>
  request<NotificationChannel>('/api/v1/notification-channels', {
    method: 'POST',
    body: JSON.stringify(req),
  });

export const updateNotificationChannel = (id: string, req: UpsertChannelRequest) =>
  request<NotificationChannel>(`/api/v1/notification-channels/${id}`, {
    method: 'PUT',
    body: JSON.stringify(req),
  });

export const deleteNotificationChannel = (id: string) =>
  request<void>(`/api/v1/notification-channels/${id}`, { method: 'DELETE' });

// Asset sets (ADR 003 R1c-b)
import type { AssetSet, AssetSetPreview, OneShotScan } from './types';
export const listAssetSets = () => request<AssetSet[]>('/api/v1/asset-sets');

export interface UpsertAssetSetRequest {
  name: string;
  description?: string;
  predicate: Record<string, unknown>;
}

export const createAssetSet = (req: UpsertAssetSetRequest) =>
  request<AssetSet>('/api/v1/asset-sets', { method: 'POST', body: JSON.stringify(req) });

export const updateAssetSet = (id: string, req: UpsertAssetSetRequest) =>
  request<AssetSet>(`/api/v1/asset-sets/${id}`, { method: 'PUT', body: JSON.stringify(req) });

export const deleteAssetSet = (id: string) =>
  request<void>(`/api/v1/asset-sets/${id}`, { method: 'DELETE' });

export const previewAssetSet = (id: string) =>
  request<AssetSetPreview>(`/api/v1/asset-sets/${id}/preview`);

export const previewAssetSetAdhoc = (predicate: Record<string, unknown>) =>
  request<AssetSetPreview>('/api/v1/asset-sets/preview', {
    method: 'POST',
    body: JSON.stringify({ predicate }),
  });

// One-shot scans (ADR 003 R1c-c)
export const listOneShotScans = () => request<OneShotScan[]>('/api/v1/one-shot-scans');

export interface CreateOneShotScanRequest {
  bundle_id: string;
  agent_id: string;
  asset_set_id?: string;
  inline_predicate?: Record<string, unknown>;
  max_concurrency?: number;
  rate_limit_pps?: number;
  triggered_by?: string;
}

export const createOneShotScan = (req: CreateOneShotScanRequest) =>
  request<OneShotScan>('/api/v1/one-shot-scans', {
    method: 'POST',
    body: JSON.stringify(req),
  });

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
export const upgradeAgent = (id: string, version?: string) =>
  request<{ status: string; version: string }>(`/api/v1/agents/${id}/upgrade`, {
    method: 'POST',
    body: JSON.stringify({ version: version ?? 'latest' }),
  });
export const getAgentDownloads = () =>
  request<AgentDownloads>('/api/v1/agents/downloads');
export const createInstallToken = () =>
  request<{ install_token: string; expires_at: string }>('/api/v1/agents/install-tokens', {
    method: 'POST',
    body: '{}',
  });

// Credentials (per-target)
export const getTargetCredential = (targetId: string) =>
  request<{ set: boolean; type?: string }>(`/api/v1/targets/${targetId}/credential`);
export const putTargetCredential = (targetId: string, type: string, data: Record<string, unknown>) =>
  request<void>(`/api/v1/targets/${targetId}/credential`, {
    method: 'PUT',
    body: JSON.stringify({ type, data }),
  });
export const deleteTargetCredential = (targetId: string) =>
  request<void>(`/api/v1/targets/${targetId}/credential`, { method: 'DELETE' });
export const probeTarget = (targetId: string) =>
  request<{ ok: boolean; error?: string; detail?: string }>(
    `/api/v1/targets/${targetId}/probe`,
    { method: 'POST' },
  );
