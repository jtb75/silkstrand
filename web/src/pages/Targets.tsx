import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { listTargets, createTarget, deleteTarget, listAgents, probeTarget, updateTarget } from '../api/client';
import type { Target, TargetType, CreateTargetRequest, Agent } from '../api/types';
import CredentialModal from '../components/CredentialModal';

// Supported technologies for the target creation form. The `type` value is
// what the API stores (and what the prober / bundle matcher dispatch on).
const TECHNOLOGIES: {
  value: TargetType;
  label: string;
  defaults: Record<string, unknown>;
}[] = [
  { value: 'postgresql',        label: 'PostgreSQL',
    defaults: { host: '', port: 5432, database: 'postgres', sslmode: 'prefer' } },
  { value: 'aurora_postgresql', label: 'Aurora PostgreSQL',
    defaults: { host: '', port: 5432, database: 'postgres', sslmode: 'require' } },
  { value: 'mssql',             label: 'Microsoft SQL Server',
    defaults: { host: '', port: 1433, database: 'master', encrypt: true } },
  { value: 'mongodb',           label: 'MongoDB',
    defaults: { host: '', port: 27017, auth_source: 'admin', tls: false } },
  { value: 'mysql',             label: 'MySQL / MariaDB',
    defaults: { host: '', port: 3306, database: '' } },
  { value: 'aurora_mysql',      label: 'Aurora MySQL',
    defaults: { host: '', port: 3306, database: '' } },
];

export default function Targets() {
  const queryClient = useQueryClient();
  const [showForm, setShowForm] = useState(false);
  const [credTarget, setCredTarget] = useState<Target | null>(null);
  const [probeBusy, setProbeBusy] = useState<Record<string, boolean>>({});
  const [probeResult, setProbeResult] = useState<Record<string, { ok: boolean; msg: string }>>({});

  // Form state — technology-specific config is a free-form dict that
  // mirrors the config JSON each driver will see on the agent side.
  const [techType, setTechType] = useState<TargetType>('postgresql');
  const [identifier, setIdentifier] = useState('');
  const [agentId, setAgentId] = useState<string>('');
  const [environment, setEnvironment] = useState('');
  const [config, setConfig] = useState<Record<string, unknown>>(TECHNOLOGIES[0].defaults);

  const { data: targets, isLoading, error } = useQuery<Target[]>({
    queryKey: ['targets'],
    queryFn: listTargets,
  });

  const { data: agents } = useQuery<Agent[]>({
    queryKey: ['agents'],
    queryFn: listAgents,
  });

  const createMutation = useMutation({
    mutationFn: (req: CreateTargetRequest) => createTarget(req),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['targets'] });
      setShowForm(false);
      // Reset form
      setTechType('postgresql');
      setIdentifier('');
      setAgentId('');
      setEnvironment('');
      setConfig(TECHNOLOGIES[0].defaults);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteTarget(id),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['targets'] }); },
  });

  const assignAgentMutation = useMutation({
    mutationFn: ({ id, agentId }: { id: string; agentId: string | undefined }) =>
      updateTarget(id, { agent_id: agentId }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['targets'] }); },
  });

  function handleTechChange(newType: TargetType) {
    setTechType(newType);
    const tech = TECHNOLOGIES.find((t) => t.value === newType);
    setConfig(tech ? { ...tech.defaults } : {});
  }

  function setConfigField(key: string, value: unknown) {
    setConfig((prev) => ({ ...prev, [key]: value }));
  }

  function handleCreate(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    createMutation.mutate({
      type: techType,
      identifier,
      environment: environment || undefined,
      agent_id: agentId || undefined,
      config,
    });
  }

  async function handleProbe(id: string) {
    setProbeBusy((p) => ({ ...p, [id]: true }));
    setProbeResult((p) => ({ ...p, [id]: { ok: false, msg: 'Testing…' } }));
    try {
      const r = await probeTarget(id);
      const msg = r.ok
        ? (r.detail ? r.detail.slice(0, 80) : 'OK')
        : (r.error || 'Failed');
      setProbeResult((p) => ({ ...p, [id]: { ok: r.ok, msg } }));
    } catch (e) {
      setProbeResult((p) => ({ ...p, [id]: { ok: false, msg: (e as Error).message } }));
    } finally {
      setProbeBusy((p) => ({ ...p, [id]: false }));
    }
  }

  function handleDelete(id: string) {
    if (window.confirm('Delete this target? Credentials and scan history will be removed.')) {
      deleteMutation.mutate(id);
    }
  }

  return (
    <div>
      <div className="page-header">
        <h1>Targets</h1>
        <button className="btn btn-primary" onClick={() => setShowForm(!showForm)}>
          {showForm ? 'Cancel' : 'New Target'}
        </button>
      </div>

      {showForm && (
        <form className="form-card" onSubmit={handleCreate}>
          <div className="form-group">
            <label htmlFor="tech">Technology</label>
            <select
              id="tech"
              value={techType}
              onChange={(e) => handleTechChange(e.target.value as TargetType)}
              required
            >
              {TECHNOLOGIES.map((t) => (
                <option key={t.value} value={t.value}>{t.label}</option>
              ))}
            </select>
          </div>

          <div className="form-group">
            <label htmlFor="identifier">Name</label>
            <input
              id="identifier"
              type="text"
              required
              placeholder="e.g. studio-prod-postgres"
              value={identifier}
              onChange={(e) => setIdentifier(e.target.value)}
            />
          </div>

          <div className="form-group">
            <label htmlFor="agent">Agent</label>
            <select
              id="agent"
              value={agentId}
              onChange={(e) => setAgentId(e.target.value)}
            >
              <option value="">— none (unassigned) —</option>
              {agents?.map((a) => (
                <option key={a.id} value={a.id}>{a.name} ({a.status})</option>
              ))}
            </select>
          </div>

          <div className="form-group">
            <label htmlFor="environment">Environment</label>
            <input
              id="environment"
              type="text"
              placeholder="e.g. production, staging (optional)"
              value={environment}
              onChange={(e) => setEnvironment(e.target.value)}
            />
          </div>

          <ConnectionFields techType={techType} config={config} onChange={setConfigField} />

          <p className="muted" style={{ fontSize: 13, marginTop: -6 }}>
            After creating, click <strong>Credential</strong> on the row to set the
            username and password the agent will use to connect.
          </p>
          <button type="submit" className="btn btn-primary" disabled={createMutation.isPending}>
            {createMutation.isPending ? 'Creating…' : 'Create Target'}
          </button>
          {createMutation.error && (
            <p className="error">{(createMutation.error as Error).message}</p>
          )}
        </form>
      )}

      {isLoading && <p>Loading…</p>}
      {error && <p className="error">Failed to load targets: {(error as Error).message}</p>}
      {!isLoading && targets && targets.length === 0 && <p>No targets yet.</p>}
      {targets && targets.length > 0 && (
        <table className="table">
          <thead>
            <tr>
              <th>Technology</th>
              <th>Name</th>
              <th>Environment</th>
              <th>Agent</th>
              <th>Created</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {targets.map((t) => (
              <tr key={t.id}>
                <td><span className="badge badge-type">{techLabel(t.type)}</span></td>
                <td>{t.identifier}</td>
                <td>{t.environment || '-'}</td>
                <td>
                  <select
                    value={t.agent_id ?? ''}
                    onChange={(e) =>
                      assignAgentMutation.mutate({ id: t.id, agentId: e.target.value || undefined })
                    }
                    disabled={assignAgentMutation.isPending}
                    title="Assign an agent to this target"
                  >
                    <option value="">— unassigned —</option>
                    {agents?.map((a) => (
                      <option key={a.id} value={a.id}>{a.name} ({a.status})</option>
                    ))}
                  </select>
                </td>
                <td>{new Date(t.created_at).toLocaleString()}</td>
                <td style={{ textAlign: 'right' }}>
                  {probeResult[t.id] && (
                    <span
                      style={{
                        marginRight: 8,
                        fontSize: 12,
                        color: probeResult[t.id].ok ? '#065f46' : '#b91c1c',
                      }}
                      title={probeResult[t.id].msg}
                    >
                      {probeResult[t.id].ok
                        ? '✓ connected'
                        : `✗ ${probeResult[t.id].msg.slice(0, 40)}${probeResult[t.id].msg.length > 40 ? '…' : ''}`}
                    </span>
                  )}
                  <button
                    className="btn btn-sm"
                    style={{ marginRight: 6 }}
                    onClick={() => handleProbe(t.id)}
                    disabled={probeBusy[t.id]}
                    title="Send a lightweight connection test through the agent"
                  >
                    {probeBusy[t.id] ? 'Testing…' : 'Test'}
                  </button>
                  <button className="btn btn-sm" style={{ marginRight: 6 }} onClick={() => setCredTarget(t)}>
                    Credential
                  </button>
                  <button className="btn btn-danger btn-sm" onClick={() => handleDelete(t.id)} disabled={deleteMutation.isPending}>
                    Delete
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {credTarget && (
        <CredentialModal target={credTarget} onClose={() => setCredTarget(null)} />
      )}
    </div>
  );
}

function techLabel(type: string): string {
  const t = TECHNOLOGIES.find((x) => x.value === type);
  return t ? t.label : type;
}

function ConnectionFields({
  techType, config, onChange,
}: {
  techType: TargetType;
  config: Record<string, unknown>;
  onChange: (key: string, value: unknown) => void;
}) {
  // Shared host+port row appears for every DB engine.
  const hostPort = (
    <>
      <div className="form-group">
        <label htmlFor="host">Host</label>
        <input
          id="host"
          type="text"
          required
          placeholder="e.g. 10.0.0.5 or db.example.internal"
          value={(config.host as string) || ''}
          onChange={(e) => onChange('host', e.target.value)}
        />
      </div>
      <div className="form-group">
        <label htmlFor="port">Port</label>
        <input
          id="port"
          type="number"
          required
          value={(config.port as number) ?? ''}
          onChange={(e) => onChange('port', Number(e.target.value))}
        />
      </div>
    </>
  );

  switch (techType) {
    case 'postgresql':
    case 'aurora_postgresql':
      return (
        <>
          {hostPort}
          <div className="form-group">
            <label htmlFor="database">Database</label>
            <input
              id="database"
              type="text"
              value={(config.database as string) || ''}
              onChange={(e) => onChange('database', e.target.value)}
            />
          </div>
          <div className="form-group">
            <label htmlFor="sslmode">SSL mode</label>
            <select
              id="sslmode"
              value={(config.sslmode as string) || 'prefer'}
              onChange={(e) => onChange('sslmode', e.target.value)}
            >
              <option value="disable">disable</option>
              <option value="allow">allow</option>
              <option value="prefer">prefer</option>
              <option value="require">require</option>
              <option value="verify-ca">verify-ca</option>
              <option value="verify-full">verify-full</option>
            </select>
          </div>
        </>
      );

    case 'mssql':
      return (
        <>
          {hostPort}
          <div className="form-group">
            <label htmlFor="database">Database</label>
            <input
              id="database"
              type="text"
              value={(config.database as string) || ''}
              onChange={(e) => onChange('database', e.target.value)}
            />
          </div>
          <div className="form-group">
            <label>
              <input
                type="checkbox"
                checked={Boolean(config.encrypt)}
                onChange={(e) => onChange('encrypt', e.target.checked)}
              />
              {' '}Encrypt connection (TLS)
            </label>
          </div>
        </>
      );

    case 'mongodb':
      return (
        <>
          {hostPort}
          <div className="form-group">
            <label htmlFor="auth_source">Auth database</label>
            <input
              id="auth_source"
              type="text"
              value={(config.auth_source as string) || 'admin'}
              onChange={(e) => onChange('auth_source', e.target.value)}
            />
          </div>
          <div className="form-group">
            <label>
              <input
                type="checkbox"
                checked={Boolean(config.tls)}
                onChange={(e) => onChange('tls', e.target.checked)}
              />
              {' '}TLS
            </label>
          </div>
        </>
      );

    case 'mysql':
    case 'aurora_mysql':
      return (
        <>
          {hostPort}
          <div className="form-group">
            <label htmlFor="database">Database</label>
            <input
              id="database"
              type="text"
              value={(config.database as string) || ''}
              onChange={(e) => onChange('database', e.target.value)}
            />
          </div>
        </>
      );

    default:
      return null;
  }
}
