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

// Discovery target types (ADR 003 R1a). The identifier shape varies —
// CIDR for subnets, an A-B range for host ranges, a bare IP/hostname for
// single hosts. The agent's allowlist YAML is the ultimate gate; these
// type labels just pick the right placeholder.
type DiscoveryType = 'cidr' | 'network_range' | 'host';
const DISCOVERY_TYPES: { value: DiscoveryType; label: string; placeholder: string }[] = [
  { value: 'cidr',          label: 'CIDR',          placeholder: '10.0.0.0/16' },
  { value: 'network_range', label: 'IP range',      placeholder: '10.0.0.1-10.0.0.254' },
  { value: 'host',          label: 'Single host',   placeholder: '10.0.0.5 or db.example.com' },
];

type Kind = 'compliance' | 'discovery';

interface DiscoveryCfg {
  ports: string;
  ratePps: string;
  includeHttpx: boolean;
  includeNuclei: boolean;
}

const DISCOVERY_DEFAULTS: DiscoveryCfg = {
  ports: '',
  ratePps: '',
  includeHttpx: true,
  includeNuclei: true,
};

export default function Targets() {
  const queryClient = useQueryClient();
  const [showForm, setShowForm] = useState(false);
  const [credTarget, setCredTarget] = useState<Target | null>(null);
  const [probeBusy, setProbeBusy] = useState<Record<string, boolean>>({});
  const [probeResult, setProbeResult] = useState<Record<string, { ok: boolean; msg: string }>>({});

  // Form state — technology-specific config is a free-form dict that
  // mirrors the config JSON each driver will see on the agent side.
  const [kind, setKind] = useState<Kind>('compliance');
  const [techType, setTechType] = useState<TargetType>('postgresql');
  const [discoveryType, setDiscoveryType] = useState<DiscoveryType>('cidr');
  const [identifier, setIdentifier] = useState('');
  const [agentId, setAgentId] = useState<string>('');
  const [environment, setEnvironment] = useState('');
  const [config, setConfig] = useState<Record<string, unknown>>(TECHNOLOGIES[0].defaults);
  const [discoveryCfg, setDiscoveryCfg] = useState<DiscoveryCfg>(DISCOVERY_DEFAULTS);

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
      setKind('compliance');
      setTechType('postgresql');
      setDiscoveryType('cidr');
      setIdentifier('');
      setAgentId('');
      setEnvironment('');
      setConfig(TECHNOLOGIES[0].defaults);
      setDiscoveryCfg(DISCOVERY_DEFAULTS);
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
    if (kind === 'discovery') {
      const cfg: Record<string, unknown> = {
        include_httpx: discoveryCfg.includeHttpx,
        include_nuclei: discoveryCfg.includeNuclei,
      };
      const ports = discoveryCfg.ports.trim();
      if (ports) cfg.ports = ports;
      const rate = parseInt(discoveryCfg.ratePps, 10);
      if (!isNaN(rate) && rate > 0) cfg.rate_pps = rate;
      createMutation.mutate({
        type: discoveryType,
        identifier,
        environment: environment || undefined,
        agent_id: agentId || undefined,
        config: cfg,
      });
      return;
    }
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
            <label>Kind</label>
            <div style={{ display: 'flex', gap: 16 }}>
              <label style={{ fontWeight: 'normal' }}>
                <input
                  type="radio"
                  name="kind"
                  value="compliance"
                  checked={kind === 'compliance'}
                  onChange={() => setKind('compliance')}
                />{' '}Compliance (database engine)
              </label>
              <label style={{ fontWeight: 'normal' }}>
                <input
                  type="radio"
                  name="kind"
                  value="discovery"
                  checked={kind === 'discovery'}
                  onChange={() => { setKind('discovery'); setIdentifier(''); }}
                />{' '}Discovery (network range)
              </label>
            </div>
            {kind === 'discovery' && (
              <p className="muted" style={{ fontSize: 13, marginTop: 4 }}>
                Discovery targets are fed to naabu → httpx → nuclei when the
                Scans page kicks a discovery scan. The agent's allowlist YAML
                is the ultimate gate — anything outside it is refused.
              </p>
            )}
          </div>

          {kind === 'compliance' && (
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
          )}

          {kind === 'discovery' && (
            <div className="form-group">
              <label htmlFor="discovery-type">Discovery target type</label>
              <select
                id="discovery-type"
                value={discoveryType}
                onChange={(e) => setDiscoveryType(e.target.value as DiscoveryType)}
                required
              >
                {DISCOVERY_TYPES.map((t) => (
                  <option key={t.value} value={t.value}>{t.label}</option>
                ))}
              </select>
            </div>
          )}

          <div className="form-group">
            <label htmlFor="identifier">
              {kind === 'discovery' ? 'Target' : 'Name'}
            </label>
            <input
              id="identifier"
              type="text"
              required
              placeholder={
                kind === 'discovery'
                  ? DISCOVERY_TYPES.find((t) => t.value === discoveryType)?.placeholder
                  : 'e.g. studio-prod-postgres'
              }
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

          {kind === 'compliance' && (
            <ConnectionFields techType={techType} config={config} onChange={setConfigField} />
          )}

          {kind === 'discovery' && (
            <DiscoveryOptions value={discoveryCfg} onChange={setDiscoveryCfg} />
          )}

          {kind === 'compliance' && (
            <p className="muted" style={{ fontSize: 13, marginTop: -6 }}>
              After creating, click <strong>Credential</strong> on the row to set the
              username and password the agent will use to connect.
            </p>
          )}
          {kind === 'discovery' && (
            <p className="muted" style={{ fontSize: 13, marginTop: -6 }}>
              After creating, start a <strong>Discovery</strong> scan from the
              Scans page to populate the Assets inventory for this target.
            </p>
          )}
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
                  {!isDiscoveryType(t.type) && (
                    <>
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
                    </>
                  )}
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

function isDiscoveryType(type: string): boolean {
  return DISCOVERY_TYPES.some((d) => d.value === type);
}

function techLabel(type: string): string {
  const t = TECHNOLOGIES.find((x) => x.value === type);
  if (t) return t.label;
  const d = DISCOVERY_TYPES.find((x) => x.value === type);
  if (d) return d.label;
  return type;
}

function DiscoveryOptions({
  value, onChange,
}: {
  value: DiscoveryCfg;
  onChange: (next: DiscoveryCfg) => void;
}) {
  return (
    <>
      <div className="form-group">
        <label htmlFor="ports">Ports (optional)</label>
        <input
          id="ports"
          type="text"
          placeholder="e.g. 80,443,5432 or 1-1000 (leave blank for naabu defaults)"
          value={value.ports}
          onChange={(e) => onChange({ ...value, ports: e.target.value })}
        />
      </div>
      <div className="form-group">
        <label htmlFor="rate_pps">Rate (packets/sec, optional)</label>
        <input
          id="rate_pps"
          type="number"
          min={1}
          placeholder="agent allowlist caps this — blank uses the cap"
          value={value.ratePps}
          onChange={(e) => onChange({ ...value, ratePps: e.target.value })}
        />
      </div>
      <div className="form-group">
        <label style={{ fontWeight: 'normal' }}>
          <input
            type="checkbox"
            checked={value.includeHttpx}
            onChange={(e) => onChange({ ...value, includeHttpx: e.target.checked })}
          />{' '}Run httpx (service fingerprinting on open HTTP(S) ports)
        </label>
      </div>
      <div className="form-group">
        <label style={{ fontWeight: 'normal' }}>
          <input
            type="checkbox"
            checked={value.includeNuclei}
            onChange={(e) => onChange({ ...value, includeNuclei: e.target.checked })}
          />{' '}Run nuclei (CVE / tech detection templates)
        </label>
      </div>
    </>
  );
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
