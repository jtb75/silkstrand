import { useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  bulkCreateCredentialMappings,
  createCredentialSource,
  deleteCredentialMapping,
  deleteCredentialSource,
  listCollections,
  listCredentialMappings,
  listCredentialSources,
  updateCredentialSource,
  type CredentialSource,
  type CredentialSourceType,
} from '../api/client';

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
        description="External secret resolvers. Plumbing only -- actual secret fetch returns 501 until the resolvers ship."
        sources={vaultSources}
        allowedTypes={VAULT_TYPES}
        banner="Not yet activated -- ADR 004 C1+."
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
  banner?: string;
}

function Section({ title, description, sources, allowedTypes, supportsMappings, banner }: SectionProps) {
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
      {banner && (
        <div className="form-card" style={{ background: '#fff7ed', borderColor: '#fdba74' }}>
          <strong>{banner}</strong>
        </div>
      )}

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
  onDelete,
}: {
  source: CredentialSource;
  supportsMappings: boolean;
  onDelete: () => void;
}) {
  const { data: mappings } = useQuery({
    queryKey: ['credential-mappings'],
    queryFn: listCredentialMappings,
    enabled: supportsMappings,
  });
  const [showMapModal, setShowMapModal] = useState(false);
  const [showEditForm, setShowEditForm] = useState(false);
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
            <button className="btn btn-sm" onClick={() => setShowMapModal(true)}>
              Map to collection...
            </button>
          </td>
        )}
        <td style={{ display: 'flex', gap: 6 }}>
          <button className="btn btn-sm" onClick={() => setShowEditForm((v) => !v)}>
            {showEditForm ? 'Cancel' : 'Edit'}
          </button>
          <button className="btn btn-sm btn-danger" onClick={onDelete}>Delete</button>
        </td>
      </tr>
      {showEditForm && (
        <tr>
          <td colSpan={supportsMappings ? 7 : 6}>
            <EditSourceForm source={source} onDone={() => setShowEditForm(false)} />
          </td>
        </tr>
      )}
      {showMapModal && (
        <tr>
          <td colSpan={supportsMappings ? 7 : 6}>
            <MapToCollectionPanel
              sourceId={source.id}
              existingMappings={mapped}
              onClose={() => setShowMapModal(false)}
            />
          </td>
        </tr>
      )}
    </>
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
            <input id="edit_username" name="edit_username" type="text" required />
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
  existingMappings: Array<{ id: string; collection_id: string }>;
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
        config.aws_access_key_id = (fd.get('aws_access_key_id') as string).trim();
        const sk = (fd.get('aws_secret_access_key') as string).trim();
        if (sk) config.aws_secret_access_key = sk;
        break;
      }
      case 'hashicorp_vault': {
        config.address = (fd.get('vault_addr') as string).trim();
        const tok = (fd.get('vault_token') as string).trim();
        if (tok) config.token = tok;
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
          <Field name="aws_region" label="AWS region" required />
          <Field name="aws_access_key_id" label="Access key ID" required />
          <Field name="aws_secret_access_key" label="Secret access key" type="password" required />
        </>
      )}
      {type === 'hashicorp_vault' && (
        <>
          <Field name="vault_addr" label="Vault address" type="url" required />
          <Field name="vault_token" label="Token" type="password" required />
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
