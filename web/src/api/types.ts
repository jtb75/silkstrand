export interface Target {
  id: string;
  tenant_id: string;
  agent_id?: string;
  type: 'database' | 'host' | 'cidr' | 'cloud';
  identifier: string;
  config: Record<string, unknown>;
  environment?: string;
  created_at: string;
  updated_at: string;
}

export interface CreateTargetRequest {
  agent_id?: string;
  type: string;
  identifier: string;
  config?: Record<string, unknown>;
  environment?: string;
}

export interface UpdateTargetRequest {
  agent_id?: string;
  type?: string;
  identifier?: string;
  config?: Record<string, unknown>;
  environment?: string;
}

export interface Scan {
  id: string;
  tenant_id: string;
  agent_id?: string;
  target_id: string;
  bundle_id: string;
  status: 'pending' | 'running' | 'completed' | 'failed';
  started_at?: string;
  completed_at?: string;
  created_at: string;
  results?: ScanResult[];
  summary?: ScanSummary;
}

export interface ScanResult {
  id: string;
  scan_id: string;
  control_id: string;
  title: string;
  status: 'PASS' | 'FAIL' | 'ERROR' | 'NOT_APPLICABLE';
  severity?: string;
  evidence?: Record<string, unknown>;
  remediation?: string;
  created_at: string;
}

export interface ScanSummary {
  total: number;
  pass: number;
  fail: number;
  error: number;
  not_applicable: number;
}

export interface Bundle {
  id: string;
  name: string;
  version: string;
  framework: string;
  target_type: string;
}

// Auth / memberships

export interface User {
  id: string;
  email: string;
  last_login_at?: string;
  created_at: string;
  updated_at: string;
}

export interface Membership {
  tenant_id: string;
  tenant_name: string;
  dc_id: string;
  dc_api_url: string;
  role: 'admin' | 'member';
}

export interface ActiveTenant {
  tenant_id: string;
  data_center_id?: string;
  dc_id?: string;
  role: 'admin' | 'member';
}
