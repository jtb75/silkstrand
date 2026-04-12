export type DCEnvironment = 'stage' | 'prod';

export interface DataCenter {
  id: string;
  name: string;
  region: string;
  environment: DCEnvironment;
  api_url: string;
  status: 'active' | 'inactive';
  last_health_status: string; // 'healthy' | 'unhealthy' | 'unknown'
  last_health_check?: string;
  tenant_count: number;
  created_at: string;
  updated_at: string;
}

export interface Tenant {
  id: string;
  data_center_id: string;
  dc_tenant_id: string;
  name: string;
  status: 'active' | 'suspended';
  provisioning_status: 'pending' | 'provisioning' | 'ready' | 'failed';
  config?: Record<string, unknown>;
  created_at: string;
  updated_at: string;
  invite_results?: InviteResult[];
}

export type InviteRole = 'admin' | 'member';

export interface TenantInvite {
  email: string;
  role: InviteRole;
}

export interface InviteResult {
  email: string;
  role: InviteRole;
  status: 'invited' | 'failed';
  error?: string;
}

export interface AdminUser {
  id: string;
  email: string;
  name: string;
  role: string;
}

export interface DashboardData {
  total_data_centers: number;
  total_tenants: number;
  active_tenants: number;
  suspended_tenants: number;
  data_centers: DataCenter[];
}

export interface LoginRequest {
  email: string;
  password: string;
}

export interface LoginResponse {
  token: string;
  admin: AdminUser;
}

export interface CreateDataCenterRequest {
  name: string;
  region: string;
  environment: DCEnvironment;
  api_url: string;
  api_key: string;
}

export interface UpdateDataCenterRequest {
  name?: string;
  region?: string;
  environment?: DCEnvironment;
  api_url?: string;
  api_key?: string;
}

export interface CreateTenantRequest {
  data_center_id: string;
  name: string;
  invites?: TenantInvite[];
}

export interface UpdateTenantRequest {
  name?: string;
}

export interface UpdateTenantStatusRequest {
  status: 'active' | 'suspended';
}
