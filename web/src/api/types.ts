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
  error_message?: string;
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
  allowlist_status: AllowlistStatus;
  allowlist_checked_at?: string;
  created_at: string;
  updated_at: string;
  // P4-backend: optional inline roll-ups so the Assets table can render
  // Coverage and max-severity without a second round-trip per row.
  endpoints_count?: number;
  risk?: RiskRollup;
  coverage?: CoverageFlags;
  resource_type?: string;
}

export type AllowlistStatus = 'allowlisted' | 'out_of_policy' | 'unknown';

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

export type ChannelType = 'webhook' | 'slack' | 'email' | 'pagerduty';

export interface NotificationChannel {
  id: string;
  tenant_id: string;
  name: string;
  type: ChannelType;
  config: Record<string, unknown>;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface AssetSet {
  id: string;
  tenant_id: string;
  name: string;
  description?: string;
  predicate: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface AssetSetPreview {
  count: number;
  sample: DiscoveredAsset[];
}

// ADR 006 D5 — Collections replace asset_sets with an expanded `scope`.
export type CollectionScope = 'asset' | 'endpoint' | 'finding';
export type WidgetKind = 'list' | 'count' | 'chart';

export interface Collection {
  id: string;
  tenant_id: string;
  name: string;
  description?: string;
  scope: CollectionScope;
  predicate: Record<string, unknown>;
  is_dashboard_widget: boolean;
  widget_kind?: WidgetKind;
  widget_title?: string;
  created_at: string;
  updated_at: string;
}

export interface CollectionPreview {
  count: number;
  // Sample shape depends on scope; rendered generically in the UI.
  sample: Array<Record<string, unknown>>;
}

// Coverage + risk roll-ups returned inline on the asset detail response.
// P4-backend extends /api/v1/assets/{id} to include these alongside the
// existing DiscoveredAsset row. They are optional for backwards-compat.
export interface CoverageFlags {
  scan_configured: boolean;
  creds_mapped: boolean;
}

export interface RiskRollup {
  critical: number;
  high: number;
  medium: number;
  low: number;
  info: number;
  max_severity?: 'critical' | 'high' | 'medium' | 'low' | 'info';
  delta_since_last_scan?: number;
  top_findings?: Array<{ id: string; title: string; severity: string }>;
}

export interface CoverageRollup {
  endpoints_total: number;
  endpoints_with_scan: number;
  endpoints_with_creds: number;
  last_scan_at?: string;
  next_scan_at?: string;
  gaps: Array<{
    endpoint_id: string;
    ip: string;
    port: number;
    service?: string;
    reason: 'no_scan' | 'no_creds' | 'recent_failure';
  }>;
}

export interface AssetEndpoint {
  id: string;
  asset_id: string;
  port: number;
  protocol?: string;
  service?: string;
  version?: string;
  fingerprint?: Record<string, unknown>;
  findings_count?: number;
  coverage?: CoverageFlags;
}

export interface OneShotScan {
  id: string;
  tenant_id: string;
  bundle_id: string;
  asset_set_id?: string;
  inline_predicate?: Record<string, unknown>;
  max_concurrency: number;
  rate_limit_pps?: number;
  total_targets?: number;
  completed_targets: number;
  status: 'pending' | 'running' | 'completed' | 'failed';
  triggered_by?: string;
  created_at: string;
  dispatched_at?: string;
  completed_at?: string;
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
  // P4-backend extensions (ADR 006). Optional: older backends omit them.
  risk?: RiskRollup;
  coverage?: CoverageRollup;
  endpoints?: AssetEndpoint[];
  provenance?: {
    first_target_id?: string;
    first_agent_id?: string;
    first_scan_id?: string;
  };
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

// ADR 007 — scan definitions + findings

export type ScanDefinitionKind = 'compliance' | 'discovery';
export type ScanDefinitionScopeKind = 'asset_endpoint' | 'collection' | 'cidr';

export interface ScanDefinition {
  id: string;
  tenant_id: string;
  name: string;
  kind: ScanDefinitionKind;
  bundle_id?: string;
  scope_kind: ScanDefinitionScopeKind;
  asset_endpoint_id?: string;
  collection_id?: string;
  cidr?: string;
  agent_id?: string;
  schedule?: string; // cron; null = manual
  enabled: boolean;
  next_run_at?: string;
  last_run_at?: string;
  last_run_status?: string;
  created_at: string;
  updated_at: string;
  created_by?: string;
}

export interface ScanDefinitionCoverage {
  scan_definition_id: string;
  endpoint_count: number;
  description?: string;
}

export type FindingSourceKind =
  | 'network_vuln'
  | 'network_compliance'
  | 'bundle_compliance';

export type FindingStatus = 'open' | 'resolved' | 'suppressed';

export interface Finding {
  id: string;
  tenant_id: string;
  asset_endpoint_id: string;
  scan_id?: string;
  source_kind: FindingSourceKind;
  source: string;
  source_id?: string;
  cve_id?: string;
  severity?: string;
  title: string;
  status: FindingStatus;
  evidence?: Record<string, unknown>;
  remediation?: string;
  first_seen: string;
  last_seen: string;
  resolved_at?: string;
  // Display helpers — these may be populated server-side via joins on the
  // list handler so the UI can render "asset:port" without an extra lookup.
  asset_ip?: string;
  asset_hostname?: string;
  endpoint_port?: number;
  collection_id?: string;
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
