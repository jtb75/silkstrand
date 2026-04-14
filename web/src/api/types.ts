export type TargetType =
  | 'postgresql'
  | 'aurora_postgresql'
  | 'mssql'
  | 'mongodb'
  | 'mysql'
  | 'aurora_mysql'
  | 'host'
  | 'cidr'
  | 'cloud'
  | 'network_range';

export interface Target {
  id: string;
  tenant_id: string;
  agent_id?: string;
  type: TargetType;
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
  target_id?: string;
  bundle_id: string;
  scan_type?: 'compliance' | 'discovery';
  status: 'pending' | 'running' | 'completed' | 'failed';
  started_at?: string;
  completed_at?: string;
  created_at: string;
  results?: ScanResult[];
  summary?: ScanSummary;
}

// ADR 003 R1a — Asset / discovery types.
export type AssetSource = 'manual' | 'discovered';

export interface CVE {
  id: string;
  severity?: 'critical' | 'high' | 'medium' | 'low' | 'info';
  template?: string;
  found_at?: string;
}

export interface AssetSuggestion {
  rule_name: string;
  bundle_id: string;
  suggested_at: string;
}

export interface DiscoveredAsset {
  id: string;
  tenant_id: string;
  ip: string;
  port: number;
  hostname?: string;
  service?: string;
  version?: string;
  technologies: unknown;
  cves: CVE[] | unknown;
  compliance_status?: string;
  source: AssetSource;
  environment?: string;
  first_seen: string;
  last_seen: string;
  last_scan_id?: string;
  missed_scan_count: number;
  metadata?: { suggested?: AssetSuggestion[]; [k: string]: unknown };
  created_at: string;
  updated_at: string;
}

// Correlation rule (ADR 003 R1b)
export interface CorrelationRuleBody {
  match: Record<string, unknown>;
  actions: Array<{ type: string; bundle_id?: string; bundle?: string; [k: string]: unknown }>;
}

export interface CorrelationRule {
  id: string;
  tenant_id: string;
  name: string;
  version: number;
  enabled: boolean;
  trigger: 'asset_discovered' | 'asset_event';
  event_type_filter?: string;
  body: CorrelationRuleBody;
  created_at: string;
  created_by?: string;
}

export type AssetEventType =
  | 'new_asset'
  | 'asset_gone'
  | 'asset_reappeared'
  | 'new_cve'
  | 'cve_resolved'
  | 'version_changed'
  | 'port_opened'
  | 'port_closed'
  | 'compliance_pass'
  | 'compliance_fail';

export interface AssetEvent {
  id: string;
  tenant_id: string;
  asset_id: string;
  scan_id?: string;
  event_type: AssetEventType;
  severity?: string;
  payload: unknown;
  occurred_at: string;
}

export interface AssetListResponse {
  items: DiscoveredAsset[];
  page: number;
  page_size: number;
  total: number;
}

export interface AssetDetailResponse {
  asset: DiscoveredAsset;
  events: AssetEvent[];
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

export interface Agent {
  id: string;
  tenant_id: string;
  name: string;
  status: 'pending' | 'connected' | 'disconnected' | 'online' | 'offline';
  last_heartbeat?: string;
  version?: string;
  created_at: string;
}

export interface CreateAgentResponse {
  agent: Agent;
  api_key: string; // plaintext, shown once
}

export interface AgentDownloads {
  version: string;
  install_script: string;
  install_cmd: string;
  binaries: Record<string, string>;
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
  display_name: string;
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
  dc_api_url?: string; // Base URL for the DC API serving this tenant
  role: 'admin' | 'member';
}
