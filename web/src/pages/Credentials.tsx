import { useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  bulkCreateCredentialMappings,
  bulkCreateEndpointMappings,
  bulkCreateAssetMappings,
  createCredentialSource,
  deleteCredentialMapping,
  deleteCredentialSource,
  listAgents,
  listAssetEndpoints,
  listAssets,
  listCollections,
  listCredentialMappings,
  listCredentialSources,
  testCredentialSource,
  updateCredentialSource,
  type CredentialMapping,
  type CredentialSource,
  type CredentialSourceType,
} from '../api/client';
import type { Agent } from '../api/types';

// Consolidated Credentials surface (P5-b). Three sections, one page:
//
//   - DB / host auth   -> type='static'
//   - Integrations     -> type in (slack, webhook, email, pagerduty)
//   - Vaults           -> type in (aws_secrets_manager, hashicorp_vault, cyberark)
//
// Integrations absorbs the old NotificationChannels page 1:1. Vaults is
// plumbing-only -- the backend persists config JSONB but the resolver
// fetch-path returns 501 until ADR 004 C1+ resolvers ship.

const INTEGRATION_TYPES: CredentialSourceType[] = ['slack', 'webhook', 'email', 'pagerduty'];
const VAULT_TYPES: CredentialSourceType[] = [
  'aws_secrets_manager',
  'hashicorp_vault',
  'cyberark',
];

export default function Credentials() {
  const { data: sources, isLoading, error } = useQuery({
    queryKey: ['credential-sources'],
    queryFn: () => listCredentialSources(),
  });

  const byType = useMemo(() => {
    const g: Record<string, CredentialSource[]> = {};
    for (const s of sources ?? []) (g[s.type] ??= []).push(s);
    return g;
  }, [sources]);

  const staticSources = byType['static'] ?? [];
  const integrationSources = INTEGRATION_TYPES.flatMap((t) => byType[t] ?? []);
  const vaultSources = VAULT_TYPES.flatMap((t) => byType[t] ?? []);

  return (
    <div>
      {isLoading && <p>Loading...</p>}
      {error && <p className="error">{(error as Error).message}</p>}

      <Section
        title="DB / host auth"
        description="Static credentials used to authenticate compliance scans against databases and hosts."
        sources={staticSources}
        allowedTypes={['static']}
        supportsMappings
      />

      <Section
        title="Integrations"
        description="Notification channels and webhooks. Triggered by correlation-rule actions."
        sources={integrationSources}
        allowedTypes={INTEGRATION_TYPES}
      />

      <Section
        title="Vaults"
        description="External secret resolvers. AWS Secrets Manager and HashiCorp Vault are live; CyberArk is coming soon."
        sources={vaultSources}
        allowedTypes={VAULT_TYPES}
        supportsMappings
        testableTypes={['aws_secrets_manager', 'hashicorp_vault']}
      />
    </div>
  );
}

interface SectionProps {
  title: string;
  description: string;
  sources: CredentialSource[];
  allowedTypes: CredentialSourceType[];
  supportsMappings?: boolean;
  testableTypes?: string[];
}

function Section({ title, description, sources, allowedTypes, supportsMappings, testableTypes }: SectionProps) {
  const queryClient = useQueryClient();
  const [showForm, setShowForm] = useState(false);

  const deleteMut = useMutation({
    mutationFn: deleteCredentialSource,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['credential-sources'] }),
  });

  return (
    <section style={{ marginTop: 24 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
        <h2 style={{ margin: 0 }}>{title}</h2>
        <button className="btn btn-sm" onClick={() => setShowForm((v) => !v)}>
          {showForm ? 'Cancel' : '+ New'}
        </button>
      </div>
      <p className="muted" style={{ marginTop: 4 }}>{description}</p>

      {showForm && (
        <CredentialSourceForm
          allowedTypes={allowedTypes}
          onDone={() => setShowForm(false)}
        />
      )}

      {sources.length === 0 ? (
        <p className="muted" style={{ marginTop: 12 }}>None configured.</p>
      ) : (
        <table className="table" style={{ marginTop: 12 }}>
          <thead>
            <tr>
              <th>Name</th>
              <th>Type</th>
              <th>Config</th>
              <th>Created</th>
              {supportsMappings && <th>Mappings</th>}
              <th></th>
            </tr>
          </thead>
          <tbody>
            {sources.map((s) => (
              <SourceRow
                key={s.id}
                source={s}
                supportsMappings={!!supportsMappings}
                testable={testableTypes?.includes(s.type) ?? false}
                onDelete={() => {
                  if (!window.confirm(`Delete ${s.name || s.type} credential source?`)) return;
                  deleteMut.mutate(s.id);
                }}
              />
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}

function SourceRow({
  source,
  supportsMappings,
  testable,
  onDelete,
}: {
  source: CredentialSource;
  supportsMappings: boolean;
  testable: boolean;
  onDelete: () => void;
}) {
  const { data: mappings } = useQuery({
    queryKey: ['credential-mappings'],
    queryFn: listCredentialMappings,
    enabled: supportsMappings,
  });
  const [mapScope, setMapScope] = useState<'collection' | 'asset_endpoint' | 'asset' | null>(null);
  const [showEditForm, setShowEditForm] = useState(false);
  const [showTestModal, setShowTestModal] = useState(false);
  const mapped = (mappings ?? []).filter((m) => m.credential_source_id === source.id);

  return (
    <>
      <tr>
        <td>{source.name || <span className="muted">--</span>}</td>
        <td><span className={`badge badge-type-${source.type}`}>{source.type}</span></td>
        <td style={{ fontFamily: 'monospace', fontSize: 12 }}>
          {renderConfigSummary(source)}
        </td>
        <td style={{ fontSize: 12 }}>{new Date(source.created_at).toLocaleDateString()}</td>
        {supportsMappings && (
          <td>
            <span style={{ marginRight: 8 }}>{mapped.length} mapped</span>
            <select
              value=""
              onChange={(e) => setMapScope(e.target.value as 'collection' | 'asset_endpoint' | 'asset')}
              style={{ fontSize: 12 }}
            >
              <option value="">Map to...</option>
              <option value="asset_endpoint">Endpoint</option>
              <option value="asset">Asset</option>
              <option value="collection">Collection</option>
            </select>
          </td>
        )}
        <td style={{ display: 'flex', gap: 6 }}>
          {testable && (
            <button
              className="btn btn-sm"
              onClick={() => setShowTestModal(true)}
            >
              Test
            </button>
          )}
          <button className="btn btn-sm" onClick={() => setShowEditForm((v) => !v)}>
            {showEditForm ? 'Cancel' : 'Edit'}
          </button>
          <button className="btn btn-sm btn-danger" onClick={onDelete}>Delete</button>
        </td>
      </tr>
      {showTestModal && (
        <tr>
          <td colSpan={supportsMappings ? 6 : 5}>
            <TestCredentialModal
              source={source}
              onClose={() => setShowTestModal(false)}
            />
          </td>
        </tr>
      )}
      {showEditForm && (
        <tr>
          <td colSpan={supportsMappings ? 7 : 6}>
            <EditSourceForm source={source} onDone={() => setShowEditForm(false)} />
          </td>
        </tr>
      )}
      {mapScope === 'collection' && (
        <tr>
          <td colSpan={supportsMappings ? 7 : 6}>
            <MapToCollectionPanel
              sourceId={source.id}
              existingMappings={mapped.filter((m) => m.scope_kind === 'collection')}
              onClose={() => setMapScope(null)}
            />
          </td>
        </tr>
      )}
      {mapScope === 'asset_endpoint' && (
        <tr>
          <td colSpan={supportsMappings ? 7 : 6}>
            <MapToEndpointPanel
              sourceId={source.id}
              existingMappings={mapped.filter((m) => m.scope_kind === 'asset_endpoint')}
              onClose={() => setMapScope(null)}
            />
          </td>
        </tr>
      )}
      {mapScope === 'asset' && (
        <tr>
          <td colSpan={supportsMappings ? 7 : 6}>
            <MapToAssetPanel
              sourceId={source.id}
              existingMappings={mapped.filter((m) => m.scope_kind === 'asset')}
              onClose={() => setMapScope(null)}
            />
          </td>
        </tr>
      )}
    </>
  );
}

// Types that should default to agent-side testing (on-prem resolvers).
const AGENT_DEFAULT_TYPES: string[] = ['hashicorp_vault', 'cyberark'];
// Types that should default to server-side testing.
const SERVER_DEFAULT_TYPES: string[] = ['aws_secrets_manager', 'static'];

function TestCredentialModal({
  source,
  onClose,
}: {
  source: CredentialSource;
  onClose: () => void;
}) {
  const defaultMode = AGENT_DEFAULT_TYPES.includes(source.type) ? 'agent' : 'server';
  const [mode, setMode] = useState<'server' | 'agent'>(defaultMode);
  const [selectedAgent, setSelectedAgent] = useState<string>('');
  const [testResult, setTestResult] = useState<{
    success: boolean;
    username?: string;
    error?: string;
    hint?: string;
    duration_ms?: number;
  } | null>(null);

  const { data: agents } = useQuery({
    queryKey: ['agents'],
    queryFn: listAgents,
    enabled: mode === 'agent',
  });

  const connectedAgents = (agents ?? []).filter(
    (a: Agent) => a.status === 'connected' || a.status === 'online',
  );

  const testMut = useMutation({
    mutationFn: () =>
      testCredentialSource(source.id, mode === 'agent' ? selectedAgent : undefined),
    onSuccess: (data) => setTestResult(data),
    onError: (e) => setTestResult({ success: false, error: (e as Error).message }),
  });

  const canRun = mode === 'server' || (mode === 'agent' && selectedAgent !== '');

  return (
    <div style={{
      border: '1px solid var(--border-color, #e2e8f0)',
      borderRadius: 6,
      padding: 16,
      background: 'var(--surface-color, #fff)',
      marginTop: 4,
      marginBottom: 4,
    }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
        <strong>Test Credential</strong>
        <button className="btn btn-sm" onClick={onClose} style={{ fontSize: 12 }}>Close</button>
      </div>

      <div style={{ marginBottom: 12 }}>
        <div style={{ fontSize: 13, marginBottom: 6, fontWeight: 500 }}>Test from:</div>
        <label style={{ display: 'block', cursor: 'pointer', marginBottom: 4 }}>
          <input
            type="radio"
            name={`test-mode-${source.id}`}
            checked={mode === 'server'}
            onChange={() => { setMode('server'); setTestResult(null); }}
            style={{ marginRight: 6 }}
          />
          Server
          {SERVER_DEFAULT_TYPES.includes(source.type) && <span className="muted" style={{ fontSize: 11, marginLeft: 4 }}>(recommended)</span>}
        </label>
        <label style={{ display: 'block', cursor: 'pointer' }}>
          <input
            type="radio"
            name={`test-mode-${source.id}`}
            checked={mode === 'agent'}
            onChange={() => { setMode('agent'); setTestResult(null); }}
            style={{ marginRight: 6 }}
          />
          Agent
          {AGENT_DEFAULT_TYPES.includes(source.type) && <span className="muted" style={{ fontSize: 11, marginLeft: 4 }}>(recommended for on-prem)</span>}
        </label>
      </div>

      {mode === 'agent' && (
        <div style={{ marginBottom: 12 }}>
          <label style={{ fontSize: 13, fontWeight: 500 }}>Agent:</label>
          <select
            value={selectedAgent}
            onChange={(e) => setSelectedAgent(e.target.value)}
            style={{ display: 'block', marginTop: 4, width: '100%', maxWidth: 300 }}
          >
            <option value="">Select an agent...</option>
            {connectedAgents.map((a: Agent) => (
              <option key={a.id} value={a.id}>
                {a.name || a.id.slice(0, 8)} (connected)
              </option>
            ))}
          </select>
          {connectedAgents.length === 0 && agents && (
            <p className="muted" style={{ fontSize: 12, marginTop: 4 }}>
              No connected agents available.
            </p>
          )}
        </div>
      )}

      <button
        className="btn btn-primary btn-sm"
        disabled={testMut.isPending || !canRun}
        onClick={() => { setTestResult(null); testMut.mutate(); }}
      >
        {testMut.isPending ? 'Testing...' : 'Run Test'}
      </button>

      {testResult && (
        <div style={{
          marginTop: 12,
          padding: '8px 12px',
          fontSize: 13,
          background: testResult.success ? '#f0fdf4' : '#fef2f2',
          borderLeft: `3px solid ${testResult.success ? '#22c55e' : '#ef4444'}`,
          borderRadius: 4,
        }}>
          {testResult.success
            ? <>Success{testResult.username ? ` -- username: ${testResult.username}` : ''}</>
            : <>Failed: {testResult.error}</>}
          {testResult.duration_ms != null && (
            <span className="muted" style={{ marginLeft: 8, fontSize: 11 }}>
              ({testResult.duration_ms}ms)
            </span>
          )}
          {testResult.hint && (
            <p className="muted" style={{ fontSize: 11, marginTop: 4 }}>{testResult.hint}</p>
          )}
        </div>
      )}
    </div>
  );
}

function renderConfigSummary(s: CredentialSource): string {
  const cfg = (s.config ?? {}) as Record<string, unknown>;
  if (s.type === 'static') {
    const t = typeof cfg.type === 'string' ? cfg.type : 'db';
    return `type=${t}, password=${'*'.repeat(8)}`;
  }
  if (s.type === 'webhook') {
    const url = typeof cfg.url === 'string' ? cfg.url : '-';
    return `url=${url}${cfg.secret === '(set)' ? ' + secret' : ''}`;
  }
  if (s.type === 'slack') {
    return cfg.webhook_url === '(set)' ? 'webhook configured' : '--';
  }
  if (s.type === 'aws_secrets_manager') {
    const region = typeof cfg.region === 'string' ? cfg.region : '-';
    const arn = typeof cfg.secret_arn === 'string' ? cfg.secret_arn : '-';
    const truncatedArn = arn.length > 40 ? arn.slice(0, 37) + '...' : arn;
    return `region=${region}, arn=${truncatedArn}`;
  }
  if (s.type === 'hashicorp_vault') {
    const url = typeof cfg.vault_url === 'string' ? cfg.vault_url : '-';
    const path = typeof cfg.secret_path === 'string' ? cfg.secret_path : '-';
    const truncatedPath = path.length > 30 ? path.slice(0, 27) + '...' : path;
    return `url=${url}, path=${truncatedPath}`;
  }
  return Object.entries(cfg)
    .map(([k, v]) => `${k}=${v === '(set)' ? '(set)' : JSON.stringify(v)}`)
    .join(', ') || '--';
}

// ---------------------------------------------------------------
// Edit form for existing credential sources
// ---------------------------------------------------------------

function EditSourceForm({
  source,
  onDone,
}: {
  source: CredentialSource;
  onDone: () => void;
}) {
  const queryClient = useQueryClient();
  const [err, setErr] = useState<string | null>(null);

  const updateMut = useMutation({
    mutationFn: ({ name, config }: { name: string; config: Record<string, unknown> }) =>
      updateCredentialSource(source.id, { name, config }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['credential-sources'] });
      onDone();
    },
    onError: (e) => setErr((e as Error).message),
  });

  function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setErr(null);
    const fd = new FormData(e.currentTarget);
    const name = (fd.get('edit_name') as string).trim();

    if (source.type === 'static') {
      const username = (fd.get('edit_username') as string).trim();
      const password = (fd.get('edit_password') as string).trim();
      if (!username) {
        setErr('Username is required.');
        return;
      }
      // Blank password = keep existing (backend preserves)
      updateMut.mutate({
        name,
        config: { username, password: password || '' },
      });
      return;
    }

    // Non-static: gather config from form
    const config: Record<string, unknown> = {};
    switch (source.type) {
      case 'webhook': {
        config.url = (fd.get('webhook_url') as string).trim();
        const secret = (fd.get('webhook_secret') as string).trim();
        if (secret) config.secret = secret;
        break;
      }
      case 'slack': {
        const url = (fd.get('slack_url') as string).trim();
        if (url) config.webhook_url = url;
        break;
      }
      case 'email': {
        config.smtp_host = (fd.get('smtp_host') as string).trim();
        config.smtp_user = (fd.get('smtp_user') as string).trim();
        config.smtp_password = (fd.get('smtp_password') as string).trim();
        config.from = (fd.get('email_from') as string).trim();
        break;
      }
      case 'pagerduty': {
        config.routing_key = (fd.get('pd_routing_key') as string).trim();
        break;
      }
      case 'aws_secrets_manager': {
        config.region = (fd.get('aws_region') as string).trim();
        config.secret_arn = (fd.get('aws_secret_arn') as string).trim();
        const roleArn = (fd.get('aws_role_arn') as string).trim();
        if (roleArn) config.role_arn = roleArn;
        config.secret_key_username = (fd.get('aws_key_username') as string).trim() || 'username';
        config.secret_key_password = (fd.get('aws_key_password') as string).trim() || 'password';
        break;
      }
      case 'hashicorp_vault': {
        config.vault_url = (fd.get('vault_url') as string).trim();
        config.auth_method = 'token';
        const tok = (fd.get('vault_token') as string).trim();
        if (tok) config.token = tok;
        config.secret_path = (fd.get('vault_secret_path') as string).trim();
        config.secret_key_username = (fd.get('vault_key_username') as string).trim() || 'username';
        config.secret_key_password = (fd.get('vault_key_password') as string).trim() || 'password';
        const ns = (fd.get('vault_namespace') as string).trim();
        if (ns) config.namespace = ns;
        config.tls_skip_verify = (fd.get('vault_tls_skip') as string) === 'on';
        break;
      }
      default:
        break;
    }
    updateMut.mutate({ name, config });
  }

  const cfg = (source.config ?? {}) as Record<string, unknown>;

  return (
    <form className="form-card" style={{ marginTop: 8 }} onSubmit={handleSubmit}>
      <div className="form-group">
        <label htmlFor="edit_name">Name</label>
        <input id="edit_name" name="edit_name" type="text" defaultValue={source.name} />
      </div>

      {source.type === 'static' && (
        <>
          <div className="form-group">
            <label htmlFor="edit_username">Username</label>
            <input id="edit_username" name="edit_username" type="text" required
              defaultValue={(source.config as Record<string, unknown>)?.username as string ?? ''} />
          </div>
          <div className="form-group">
            <label htmlFor="edit_password">Password (leave blank to keep existing)</label>
            <input id="edit_password" name="edit_password" type="password" />
          </div>
        </>
      )}
      {source.type === 'webhook' && (
        <>
          <Field name="webhook_url" label="URL" type="url" defaultValue={cfg.url as string} required />
          <Field name="webhook_secret" label="Signing secret (blank = keep)" type="password" />
        </>
      )}
      {source.type === 'slack' && (
        <Field name="slack_url" label="Slack webhook URL (blank = keep)" type="url" />
      )}
      {source.type === 'email' && (
        <>
          <Field name="smtp_host" label="SMTP host" defaultValue={cfg.smtp_host as string} required />
          <Field name="smtp_user" label="SMTP user" defaultValue={cfg.smtp_user as string} required />
          <Field name="smtp_password" label="SMTP password (blank = keep)" type="password" />
          <Field name="email_from" label="From address" type="email" defaultValue={cfg.from as string} required />
        </>
      )}
      {source.type === 'pagerduty' && (
        <Field name="pd_routing_key" label="Routing key (blank = keep)" type="password" />
      )}
      {source.type === 'aws_secrets_manager' && (
        <>
          <Field name="aws_region" label="AWS region" defaultValue={cfg.region as string} required />
          <Field name="aws_secret_arn" label="Secret ARN" defaultValue={cfg.secret_arn as string} required />
          <Field name="aws_role_arn" label="Role ARN (optional)" defaultValue={cfg.role_arn as string} />
          <Field name="aws_key_username" label="Username key" defaultValue={(cfg.secret_key_username as string | undefined) ?? 'username'} />
          <Field name="aws_key_password" label="Password key" defaultValue={(cfg.secret_key_password as string | undefined) ?? 'password'} />
        </>
      )}
      {source.type === 'hashicorp_vault' && (
        <>
          <Field name="vault_url" label="Vault URL" defaultValue={cfg.vault_url as string} required />
          <Field name="vault_token" label="Token (blank = keep existing)" type="password" />
          <Field name="vault_secret_path" label="Secret path" defaultValue={cfg.secret_path as string} required />
          <Field name="vault_key_username" label="Username key" defaultValue={(cfg.secret_key_username as string | undefined) ?? 'username'} />
          <Field name="vault_key_password" label="Password key" defaultValue={(cfg.secret_key_password as string | undefined) ?? 'password'} />
          <Field name="vault_namespace" label="Namespace (optional)" defaultValue={cfg.namespace as string} />
          <div className="form-group">
            <label style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
              <input type="checkbox" name="vault_tls_skip" defaultChecked={!!cfg.tls_skip_verify} />
              Skip TLS verification
            </label>
          </div>
        </>
      )}

      {err && <p className="error">{err}</p>}
      <button className="btn btn-primary" disabled={updateMut.isPending}>
        {updateMut.isPending ? 'Saving...' : 'Save'}
      </button>
    </form>
  );
}

function MapToCollectionPanel({
  sourceId,
  existingMappings,
  onClose,
}: {
  sourceId: string;
  existingMappings: CredentialMapping[];
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const { data: collections } = useQuery({
    queryKey: ['collections'],
    queryFn: () => listCollections(),
  });
  const [selected, setSelected] = useState<Set<string>>(new Set());

  const endpointCollections = (collections ?? []).filter((c) => c.scope === 'endpoint');

  const bulkMut = useMutation({
    mutationFn: (ids: string[]) => bulkCreateCredentialMappings(sourceId, ids),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['credential-mappings'] });
      onClose();
    },
  });

  const unmapMut = useMutation({
    mutationFn: deleteCredentialMapping,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['credential-mappings'] }),
  });

  function toggle(id: string) {
    setSelected((prev) => {
      const n = new Set(prev);
      if (n.has(id)) n.delete(id); else n.add(id);
      return n;
    });
  }

  return (
    <div className="form-card" style={{ marginTop: 8 }}>
      <div style={{ display: 'flex', justifyContent: 'space-between' }}>
        <strong>Map credential to endpoint-scoped collections</strong>
        <button className="btn btn-sm" onClick={onClose}>Close</button>
      </div>
      {endpointCollections.length === 0 && (
        <p className="muted">No endpoint-scoped collections defined.</p>
      )}
      <ul style={{ listStyle: 'none', padding: 0, margin: '8px 0' }}>
        {endpointCollections.map((c) => {
          const mapping = existingMappings.find((m) => m.collection_id === c.id);
          const already = !!mapping;
          return (
            <li key={c.id} style={{ padding: '4px 0', display: 'flex', gap: 8, alignItems: 'center' }}>
              <label style={{ display: 'flex', gap: 8, alignItems: 'center', flex: 1 }}>
                <input
                  type="checkbox"
                  disabled={already}
                  checked={already || selected.has(c.id)}
                  onChange={() => toggle(c.id)}
                />
                <span>{c.name}</span>
                {already && <span className="muted">(mapped)</span>}
              </label>
              {already && (
                <button
                  className="btn btn-sm btn-danger"
                  disabled={unmapMut.isPending}
                  onClick={() => unmapMut.mutate(mapping.id)}
                >
                  Unmap
                </button>
              )}
            </li>
          );
        })}
      </ul>
      {bulkMut.error && <p className="error">{(bulkMut.error as Error).message}</p>}
      <div style={{ display: 'flex', gap: 8 }}>
        <button
          className="btn btn-primary"
          disabled={selected.size === 0 || bulkMut.isPending}
          onClick={() => bulkMut.mutate(Array.from(selected))}
        >
          {bulkMut.isPending ? 'Mapping...' : `Map ${selected.size} collection(s)`}
        </button>
        {unmapMut.isPending && <span className="muted">Unmapping...</span>}
      </div>
    </div>
  );
}

function MapToEndpointPanel({
  sourceId,
  existingMappings,
  onClose,
}: {
  sourceId: string;
  existingMappings: CredentialMapping[];
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const [search, setSearch] = useState('');
  const { data: endpoints } = useQuery({
    queryKey: ['asset-endpoints', { q: search }],
    queryFn: () => listAssetEndpoints({ q: search || undefined, page: 1, page_size: 50 }),
    enabled: search.length >= 1,
  });
  const [selected, setSelected] = useState<Set<string>>(new Set());

  // Resolve mapped endpoint UUIDs to host:port labels.
  const mappedEndpointIds = existingMappings
    .filter((m) => m.scope_kind === 'asset_endpoint' && m.asset_endpoint_id)
    .map((m) => m.asset_endpoint_id!);
  const { data: mappedEndpointLabels } = useQuery({
    queryKey: ['mapped-endpoint-labels', mappedEndpointIds],
    queryFn: async () => {
      const all = await listAssetEndpoints({ page_size: 500 });
      const map = new Map<string, string>();
      for (const ep of all.items) {
        map.set(ep.id, `${ep.host || ep.ip}:${ep.port}`);
      }
      return map;
    },
    enabled: mappedEndpointIds.length > 0,
  });

  const bulkMut = useMutation({
    mutationFn: (ids: string[]) => bulkCreateEndpointMappings(sourceId, ids),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['credential-mappings'] });
      onClose();
    },
  });

  const unmapMut = useMutation({
    mutationFn: deleteCredentialMapping,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['credential-mappings'] }),
  });

  const items = endpoints?.items ?? [];
  const mappedIds = new Set(existingMappings.map((m) => m.asset_endpoint_id).filter(Boolean));

  return (
    <div className="form-card" style={{ marginTop: 8 }}>
      <div style={{ display: 'flex', justifyContent: 'space-between' }}>
        <strong>Map credential to endpoints</strong>
        <button className="btn btn-sm" onClick={onClose}>Close</button>
      </div>
      {existingMappings.filter(m => m.scope_kind === 'asset_endpoint').length > 0 && (
        <>
          <p style={{ margin: '8px 0 4px', fontWeight: 600, fontSize: 13 }}>Current mappings</p>
          <ul style={{ listStyle: 'none', padding: 0, margin: '0 0 12px' }}>
            {existingMappings.filter(m => m.scope_kind === 'asset_endpoint' && m.asset_endpoint_id).map(m => (
              <li key={m.id} style={{ padding: '4px 0', display: 'flex', gap: 8, alignItems: 'center' }}>
                <span style={{ flex: 1 }}>{mappedEndpointLabels?.get(m.asset_endpoint_id!) ?? `${m.asset_endpoint_id?.slice(0, 8)}…`} (mapped)</span>
                <button
                  className="btn btn-sm btn-danger"
                  disabled={unmapMut.isPending}
                  onClick={() => unmapMut.mutate(m.id)}
                >
                  Unmap
                </button>
              </li>
            ))}
          </ul>
        </>
      )}
      <p style={{ margin: '8px 0 4px', fontWeight: 600, fontSize: 13 }}>Add mapping</p>
      <input
        type="search"
        placeholder="Search endpoints by host, IP, service..."
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        style={{ width: '100%' }}
      />
      {search.length >= 1 && items.length === 0 && (
        <p className="muted" style={{ marginTop: 8 }}>No endpoints found.</p>
      )}
      {items.length > 0 && (
        <ul style={{ listStyle: 'none', padding: 0, margin: '8px 0', maxHeight: 300, overflowY: 'auto' }}>
          {items.map((ep) => {
            const already = mappedIds.has(ep.id);
            const mapping = already ? existingMappings.find((m) => m.asset_endpoint_id === ep.id) : null;
            return (
              <li key={ep.id} style={{ padding: '4px 0', display: 'flex', gap: 8, alignItems: 'center' }}>
                <label style={{ display: 'flex', gap: 8, alignItems: 'center', flex: 1 }}>
                  <input
                    type="checkbox"
                    disabled={already}
                    checked={already || selected.has(ep.id)}
                    onChange={() => {
                      setSelected((prev) => {
                        const n = new Set(prev);
                        if (n.has(ep.id)) n.delete(ep.id); else n.add(ep.id);
                        return n;
                      });
                    }}
                  />
                  <span>{ep.host || ep.ip}:{ep.port}</span>
                  {ep.service && <span className="muted">({ep.service})</span>}
                  {already && <span className="muted">(mapped)</span>}
                </label>
                {already && mapping && (
                  <button
                    className="btn btn-sm btn-danger"
                    disabled={unmapMut.isPending}
                    onClick={() => unmapMut.mutate(mapping.id)}
                  >
                    Unmap
                  </button>
                )}
              </li>
            );
          })}
        </ul>
      )}
      {bulkMut.error && <p className="error">{(bulkMut.error as Error).message}</p>}
      <div style={{ display: 'flex', gap: 8, marginTop: 8 }}>
        <button
          className="btn btn-primary"
          disabled={selected.size === 0 || bulkMut.isPending}
          onClick={() => bulkMut.mutate(Array.from(selected))}
        >
          {bulkMut.isPending ? 'Mapping...' : `Map ${selected.size} endpoint(s)`}
        </button>
        {unmapMut.isPending && <span className="muted">Unmapping...</span>}
      </div>
    </div>
  );
}

function MapToAssetPanel({
  sourceId,
  existingMappings,
  onClose,
}: {
  sourceId: string;
  existingMappings: CredentialMapping[];
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const [search, setSearch] = useState('');
  const { data: assets } = useQuery({
    queryKey: ['assets', { q: search }],
    queryFn: () => listAssets({ q: search || undefined, page: 1, page_size: 50 }),
    enabled: search.length >= 1,
  });
  const [selected, setSelected] = useState<Set<string>>(new Set());

  const bulkMut = useMutation({
    mutationFn: (ids: string[]) => bulkCreateAssetMappings(sourceId, ids),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['credential-mappings'] });
      onClose();
    },
  });

  const unmapMut = useMutation({
    mutationFn: deleteCredentialMapping,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['credential-mappings'] }),
  });

  const items = assets?.items ?? [];
  const mappedIds = new Set(existingMappings.map((m) => m.asset_id).filter(Boolean));

  return (
    <div className="form-card" style={{ marginTop: 8 }}>
      <div style={{ display: 'flex', justifyContent: 'space-between' }}>
        <strong>Map credential to assets</strong>
        <button className="btn btn-sm" onClick={onClose}>Close</button>
      </div>
      <input
        type="search"
        placeholder="Search assets by hostname, IP..."
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        style={{ width: '100%', marginTop: 8 }}
      />
      {search.length >= 1 && items.length === 0 && (
        <p className="muted" style={{ marginTop: 8 }}>No assets found.</p>
      )}
      {items.length > 0 && (
        <ul style={{ listStyle: 'none', padding: 0, margin: '8px 0', maxHeight: 300, overflowY: 'auto' }}>
          {items.map((a) => {
            const already = mappedIds.has(a.id);
            const mapping = already ? existingMappings.find((m) => m.asset_id === a.id) : null;
            return (
              <li key={a.id} style={{ padding: '4px 0', display: 'flex', gap: 8, alignItems: 'center' }}>
                <label style={{ display: 'flex', gap: 8, alignItems: 'center', flex: 1 }}>
                  <input
                    type="checkbox"
                    disabled={already}
                    checked={already || selected.has(a.id)}
                    onChange={() => {
                      setSelected((prev) => {
                        const n = new Set(prev);
                        if (n.has(a.id)) n.delete(a.id); else n.add(a.id);
                        return n;
                      });
                    }}
                  />
                  <span>{a.hostname || a.primary_ip || a.ip || '-'}</span>
                  {a.resource_type && <span className="muted">({a.resource_type})</span>}
                  {already && <span className="muted">(mapped)</span>}
                </label>
                {already && mapping && (
                  <button
                    className="btn btn-sm btn-danger"
                    disabled={unmapMut.isPending}
                    onClick={() => unmapMut.mutate(mapping.id)}
                  >
                    Unmap
                  </button>
                )}
              </li>
            );
          })}
        </ul>
      )}
      {bulkMut.error && <p className="error">{(bulkMut.error as Error).message}</p>}
      <div style={{ display: 'flex', gap: 8, marginTop: 8 }}>
        <button
          className="btn btn-primary"
          disabled={selected.size === 0 || bulkMut.isPending}
          onClick={() => bulkMut.mutate(Array.from(selected))}
        >
          {bulkMut.isPending ? 'Mapping...' : `Map ${selected.size} asset(s)`}
        </button>
        {unmapMut.isPending && <span className="muted">Unmapping...</span>}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------
// New-source form. Slim but covers every allowed type.
// ---------------------------------------------------------------

function CredentialSourceForm({
  allowedTypes,
  onDone,
}: {
  allowedTypes: CredentialSourceType[];
  onDone: () => void;
}) {
  const queryClient = useQueryClient();
  const [type, setType] = useState<CredentialSourceType>(allowedTypes[0]);
  const [err, setErr] = useState<string | null>(null);

  const createMut = useMutation({
    mutationFn: createCredentialSource,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['credential-sources'] });
      onDone();
    },
    onError: (e) => setErr((e as Error).message),
  });

  function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setErr(null);
    const fd = new FormData(e.currentTarget);
    const name = (fd.get('source_name') as string | null)?.trim() ?? '';
    const config: Record<string, unknown> = {};
    switch (type) {
      case 'static': {
        const username = (fd.get('static_username') as string).trim();
        const password = (fd.get('static_password') as string).trim();
        if (!username || !password) {
          setErr('Username and password are required.');
          return;
        }
        config.username = username;
        config.password = password;
        break;
      }
      case 'webhook': {
        config.url = (fd.get('webhook_url') as string).trim();
        const secret = (fd.get('webhook_secret') as string).trim();
        if (secret) config.secret = secret;
        break;
      }
      case 'slack': {
        const url = (fd.get('slack_url') as string).trim();
        if (url) config.webhook_url = url;
        break;
      }
      case 'email': {
        config.smtp_host = (fd.get('smtp_host') as string).trim();
        config.smtp_user = (fd.get('smtp_user') as string).trim();
        config.smtp_password = (fd.get('smtp_password') as string).trim();
        config.from = (fd.get('email_from') as string).trim();
        break;
      }
      case 'pagerduty': {
        config.routing_key = (fd.get('pd_routing_key') as string).trim();
        break;
      }
      case 'aws_secrets_manager': {
        config.region = (fd.get('aws_region') as string).trim();
        config.secret_arn = (fd.get('aws_secret_arn') as string).trim();
        const roleArn = (fd.get('aws_role_arn') as string).trim();
        if (roleArn) config.role_arn = roleArn;
        config.secret_key_username = (fd.get('aws_key_username') as string).trim() || 'username';
        config.secret_key_password = (fd.get('aws_key_password') as string).trim() || 'password';
        if (!config.region || !config.secret_arn) {
          setErr('Region and Secret ARN are required.');
          return;
        }
        break;
      }
      case 'hashicorp_vault': {
        config.vault_url = (fd.get('vault_url') as string).trim();
        config.auth_method = 'token';
        const tok = (fd.get('vault_token') as string).trim();
        if (tok) config.token = tok;
        config.secret_path = (fd.get('vault_secret_path') as string).trim();
        config.secret_key_username = (fd.get('vault_key_username') as string).trim() || 'username';
        config.secret_key_password = (fd.get('vault_key_password') as string).trim() || 'password';
        const ns = (fd.get('vault_namespace') as string).trim();
        if (ns) config.namespace = ns;
        config.tls_skip_verify = (fd.get('vault_tls_skip') as string) === 'on';
        if (!config.vault_url || !config.secret_path) {
          setErr('Vault URL and secret path are required.');
          return;
        }
        if (!tok) {
          setErr('Token is required.');
          return;
        }
        break;
      }
      case 'cyberark': {
        config.app_id = (fd.get('cyberark_app_id') as string).trim();
        const key = (fd.get('cyberark_api_key') as string).trim();
        if (key) config.api_key = key;
        break;
      }
    }
    createMut.mutate({ name, type, config });
  }

  return (
    <form className="form-card" onSubmit={handleSubmit}>
      <div className="form-group">
        <label htmlFor="source_name">Name</label>
        <input id="source_name" name="source_name" type="text" placeholder="e.g. studio-mssql-sa" />
      </div>
      {allowedTypes.length > 1 && (
        <div className="form-group">
          <label>Type</label>
          <select value={type} onChange={(e) => setType(e.target.value as CredentialSourceType)}>
            {allowedTypes.map((t) => <option key={t} value={t}>{t}</option>)}
          </select>
        </div>
      )}
      {type === 'static' && (
        <>
          <Field name="static_username" label="Username" required />
          <Field name="static_password" label="Password" type="password" required />
        </>
      )}
      {type === 'webhook' && (
        <>
          <Field name="webhook_url" label="URL" type="url" required />
          <Field name="webhook_secret" label="Signing secret (optional)" type="password" />
        </>
      )}
      {type === 'slack' && (
        <Field name="slack_url" label="Slack webhook URL" type="url" required />
      )}
      {type === 'email' && (
        <>
          <Field name="smtp_host" label="SMTP host" required />
          <Field name="smtp_user" label="SMTP user" required />
          <Field name="smtp_password" label="SMTP password" type="password" required />
          <Field name="email_from" label="From address" type="email" required />
        </>
      )}
      {type === 'pagerduty' && (
        <Field name="pd_routing_key" label="Routing key" type="password" required />
      )}
      {type === 'aws_secrets_manager' && (
        <>
          <Field name="aws_region" label="AWS region" required defaultValue="us-east-1" />
          <Field name="aws_secret_arn" label="Secret ARN" required />
          <Field name="aws_role_arn" label="Role ARN (optional, for cross-account)" />
          <Field name="aws_key_username" label="Username key in secret JSON" defaultValue="username" />
          <Field name="aws_key_password" label="Password key in secret JSON" defaultValue="password" />
        </>
      )}
      {type === 'hashicorp_vault' && (
        <>
          <Field name="vault_url" label="Vault URL" required defaultValue="http://127.0.0.1:8200" />
          <Field name="vault_token" label="Token" type="password" required />
          <Field name="vault_secret_path" label="Secret path (e.g. secret/data/mssql-creds)" required />
          <Field name="vault_key_username" label="Username key in secret" defaultValue="username" />
          <Field name="vault_key_password" label="Password key in secret" defaultValue="password" />
          <Field name="vault_namespace" label="Namespace (optional)" />
          <div className="form-group">
            <label style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
              <input type="checkbox" name="vault_tls_skip" />
              Skip TLS verification (dev / self-signed certs)
            </label>
          </div>
        </>
      )}
      {type === 'cyberark' && (
        <>
          <Field name="cyberark_app_id" label="App ID" required />
          <Field name="cyberark_api_key" label="API key" type="password" required />
        </>
      )}
      {err && <p className="error">{err}</p>}
      <button className="btn btn-primary" disabled={createMut.isPending}>
        {createMut.isPending ? 'Creating...' : 'Create'}
      </button>
    </form>
  );
}

function Field({
  name,
  label,
  type = 'text',
  required,
  defaultValue,
}: {
  name: string;
  label: string;
  type?: string;
  required?: boolean;
  defaultValue?: string;
}) {
  return (
    <div className="form-group">
      <label htmlFor={name}>{label}</label>
      <input id={name} name={name} type={type} required={required} defaultValue={defaultValue} />
    </div>
  );
}
