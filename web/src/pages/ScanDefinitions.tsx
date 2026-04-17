import { useMemo, useState } from 'react';
import { useQuery, useMutation, useQueryClient, useQueries } from '@tanstack/react-query';
import { useToast } from '../lib/toast';
import {
  listScanDefinitions,
  createScanDefinition,
  updateScanDefinition,
  deleteScanDefinition,
  executeScanDefinition,
  enableScanDefinition,
  disableScanDefinition,
  getScanDefinitionCoverage,
  listBundles,
  listAgents,
  listCollections,
  listScans,
  type UpsertScanDefinitionRequest,
} from '../api/client';
import type {
  Scan,
  ScanDefinition,
  ScanDefinitionKind,
  ScanDefinitionScopeKind,
  Bundle,
  Agent,
  Collection,
} from '../api/types';

// Scopes the user can pick in the form. The backend's CHECK constraint
// (ADR 007 D3) enforces exactly-one, so we mirror the three options here.
const SCOPE_KINDS: { value: ScanDefinitionScopeKind; label: string }[] = [
  { value: 'asset_endpoint', label: 'Single endpoint' },
  { value: 'collection', label: 'Collection' },
  { value: 'cidr', label: 'CIDR / IP range' },
];

// Rough natural-language helper for common cron shapes — keeps the
// schedule field readable without pulling in a full cron-parse dep.
function cronHelp(expr: string): string {
  const s = expr.trim();
  if (!s) return 'manual only (no schedule)';
  const parts = s.split(/\s+/);
  if (parts.length !== 5) return 'enter a 5-field cron (min hr dom mon dow)';
  const [m, h, dom, mon, dow] = parts;
  if (m === '0' && h === '*' && dom === '*' && mon === '*' && dow === '*') return 'every hour';
  if (m === '0' && /^\*\/\d+$/.test(h) && dom === '*' && mon === '*' && dow === '*') {
    return `every ${h.slice(2)} hours`;
  }
  if (m === '0' && h === '0' && dom === '*' && mon === '*' && dow === '*') return 'daily at 00:00';
  if (m === '0' && /^\d+$/.test(h) && dom === '*' && mon === '*' && dow === '*') {
    return `daily at ${h.padStart(2, '0')}:00`;
  }
  if (m === '0' && h === '0' && dom === '*' && mon === '*' && /^\d+$/.test(dow)) {
    return `weekly on day ${dow}`;
  }
  return 'custom';
}

export default function ScanDefinitions() {
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [showForm, setShowForm] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);

  const [name, setName] = useState('');
  const [kind, setKind] = useState<ScanDefinitionKind>('compliance');
  const [scopeKind, setScopeKind] = useState<ScanDefinitionScopeKind>('collection');
  const [bundleId, setBundleId] = useState('');
  const [agentId, setAgentId] = useState('');
  const [schedule, setSchedule] = useState('');
  const [enabled, setEnabled] = useState(true);
  const [endpointId, setEndpointId] = useState('');
  const [collectionId, setCollectionId] = useState('');
  const [cidr, setCidr] = useState('');

  const { data: defs, isLoading, error } = useQuery<ScanDefinition[]>({
    queryKey: ['scan-definitions'],
    queryFn: listScanDefinitions,
  });

  const { data: bundles } = useQuery<Bundle[]>({
    queryKey: ['bundles'],
    queryFn: listBundles,
    enabled: showForm,
  });

  const { data: agents } = useQuery<Agent[]>({
    queryKey: ['agents'],
    queryFn: listAgents,
    enabled: showForm,
  });

  // Collections power the scope=collection picker. Filter to
  // endpoint-scoped collections only — compliance scans bind to
  // asset_endpoints, so asset- or finding-scoped collections don't
  // make sense as a scan target.
  const { data: collections } = useQuery<Collection[]>({
    queryKey: ['collections', { scope: 'endpoint' }],
    queryFn: () => listCollections({ scope: 'endpoint' }),
    enabled: showForm && scopeKind === 'collection',
  });

  // Scans list for deriving per-definition status chips. Invalidated
  // via SSE scan_status events in Layout.tsx so chips update in real time.
  const { data: scans } = useQuery<Scan[]>({
    queryKey: ['scans'],
    queryFn: listScans,
  });

  // Derive per-definition status chip from the latest scan(s).
  const defStatus = useMemo(() => {
    const map: Record<string, { status: string; queuedCount: number }> = {};
    if (!scans || !defs) return map;
    type ScanWithDef = Scan & { scan_definition_id?: string };
    for (const d of defs) {
      const matching = scans.filter(
        (s) => (s as ScanWithDef).scan_definition_id === d.id,
      );
      const running = matching.filter((s) => s.status === 'running');
      const queued = matching.filter((s) => s.status === 'queued');
      const latest = matching[0]; // scans come back newest-first
      if (running.length > 0) {
        map[d.id] = { status: 'running', queuedCount: 0 };
      } else if (queued.length > 0) {
        map[d.id] = { status: 'queued', queuedCount: queued.length };
      } else if (latest && latest.status === 'failed') {
        map[d.id] = { status: 'failed', queuedCount: 0 };
      } else {
        map[d.id] = { status: 'idle', queuedCount: 0 };
      }
    }
    return map;
  }, [scans, defs]);

  // Look up the latest scan for a definition to surface error messages.
  function latestScanForDef(defId: string): Scan | undefined {
    if (!scans || !defs) return undefined;
    type ScanWithDef = Scan & { scan_definition_id?: string };
    return scans.find((s) => (s as ScanWithDef).scan_definition_id === defId);
  }

  // Coverage Impact strip — fan out a per-def coverage fetch. Each row
  // answers "X covers N endpoints". Cached by definition id.
  const coverageQueries = useQueries({
    queries: (defs ?? []).map((d) => ({
      queryKey: ['scan-def-coverage', d.id],
      queryFn: () => getScanDefinitionCoverage(d.id),
      staleTime: 30_000,
    })),
  });

  function resetForm() {
    setEditingId(null);
    setName('');
    setKind('compliance');
    setScopeKind('collection');
    setBundleId('');
    setAgentId('');
    setSchedule('');
    setEnabled(true);
    setEndpointId('');
    setCollectionId('');
    setCidr('');
  }

  function closeForm() {
    setShowForm(false);
    resetForm();
  }

  const createMut = useMutation({
    mutationFn: (req: UpsertScanDefinitionRequest) => createScanDefinition(req),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['scan-definitions'] });
      closeForm();
      toast('Definition created', 'success');
    },
  });

  const updateMut = useMutation({
    mutationFn: ({ id, req }: { id: string; req: UpsertScanDefinitionRequest }) =>
      updateScanDefinition(id, req),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['scan-definitions'] });
      closeForm();
    },
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => deleteScanDefinition(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['scan-definitions'] });
      toast('Definition deleted', 'success');
    },
  });

  const executeMut = useMutation({
    mutationFn: (id: string) => executeScanDefinition(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['scans'] });
      toast('Scan dispatched', 'success');
    },
  });

  const toggleMut = useMutation({
    mutationFn: ({ id, enable }: { id: string; enable: boolean }) =>
      enable ? enableScanDefinition(id) : disableScanDefinition(id),
    onSuccess: (_data, vars) => {
      queryClient.invalidateQueries({ queryKey: ['scan-definitions'] });
      toast(`Definition ${vars.enable ? 'enabled' : 'disabled'}`, 'success');
    },
  });

  function handleEdit(d: ScanDefinition) {
    setEditingId(d.id);
    setName(d.name);
    setKind(d.kind);
    setScopeKind(d.scope_kind);
    setBundleId(d.bundle_id ?? '');
    setAgentId(d.agent_id ?? '');
    setSchedule(d.schedule ?? '');
    setEnabled(d.enabled);
    setEndpointId(d.asset_endpoint_id ?? '');
    setCollectionId(d.collection_id ?? '');
    setCidr(d.cidr ?? '');
    setShowForm(true);
  }

  function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const req: UpsertScanDefinitionRequest = {
      name: name.trim(),
      kind,
      scope_kind: scopeKind,
      agent_id: agentId || undefined,
      schedule: schedule.trim() || null,
      enabled,
      bundle_id: kind === 'compliance' ? bundleId || undefined : undefined,
      asset_endpoint_id: scopeKind === 'asset_endpoint' ? endpointId || undefined : undefined,
      collection_id: scopeKind === 'collection' ? collectionId || undefined : undefined,
      cidr: scopeKind === 'cidr' ? cidr || undefined : undefined,
    };
    if (editingId) updateMut.mutate({ id: editingId, req });
    else createMut.mutate(req);
  }

  // Reset scope-specific fields when the scope selector changes so we never
  // send multiple scope values to the API (server CHECK would reject).
  function changeScopeKind(next: ScanDefinitionScopeKind) {
    setScopeKind(next);
    setEndpointId('');
    setCollectionId('');
    setCidr('');
  }

  function scopeLabel(d: ScanDefinition): string {
    switch (d.scope_kind) {
      case 'asset_endpoint':
        return `Endpoint:${d.asset_endpoint_id?.slice(0, 8) ?? '?'}`;
      case 'collection':
        return `Collection:${d.collection_id?.slice(0, 8) ?? '?'}`;
      case 'cidr':
        return `CIDR:${d.cidr ?? '?'}`;
    }
  }

  return (
    <div>
      <div className="page-header" style={{ display: 'flex', justifyContent: 'space-between' }}>
        <h2 style={{ margin: 0 }}>Scan Definitions</h2>
        <button
          className="btn btn-primary"
          onClick={() => {
            if (showForm) closeForm();
            else setShowForm(true);
          }}
        >
          {showForm ? 'Cancel' : '+ New Scan Definition'}
        </button>
      </div>

      {showForm && (
        <form className="form-card" onSubmit={handleSubmit}>
          {editingId && <h3 style={{ marginTop: 0 }}>Edit scan definition</h3>}
          <div className="form-group">
            <label htmlFor="sd-name">Name</label>
            <input
              id="sd-name"
              type="text"
              required
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g. PG CIS daily"
            />
          </div>

          <div className="form-group">
            <label>Kind</label>
            <div style={{ display: 'flex', gap: 16 }}>
              <label style={{ fontWeight: 'normal' }}>
                <input
                  type="radio"
                  name="kind"
                  checked={kind === 'compliance'}
                  onChange={() => setKind('compliance')}
                />{' '}Compliance
              </label>
              <label style={{ fontWeight: 'normal' }}>
                <input
                  type="radio"
                  name="kind"
                  checked={kind === 'discovery'}
                  onChange={() => setKind('discovery')}
                />{' '}Discovery
              </label>
            </div>
          </div>

          <div className="form-group">
            <label>Scope</label>
            <div style={{ display: 'flex', gap: 16 }}>
              {SCOPE_KINDS.map((s) => (
                <label key={s.value} style={{ fontWeight: 'normal' }}>
                  <input
                    type="radio"
                    name="scope"
                    checked={scopeKind === s.value}
                    onChange={() => changeScopeKind(s.value)}
                  />{' '}{s.label}
                </label>
              ))}
            </div>
          </div>

          {scopeKind === 'asset_endpoint' && (
            <div className="form-group">
              <label htmlFor="sd-endpoint">Asset endpoint ID</label>
              <input
                id="sd-endpoint"
                type="text"
                placeholder="uuid of asset_endpoint"
                value={endpointId}
                onChange={(e) => setEndpointId(e.target.value)}
                required
              />
            </div>
          )}

          {scopeKind === 'collection' && (
            <div className="form-group">
              <label htmlFor="sd-collection">Collection</label>
              <select
                id="sd-collection"
                value={collectionId}
                onChange={(e) => setCollectionId(e.target.value)}
                required
              >
                <option value="">Select…</option>
                {collections?.map((c) => (
                  <option key={c.id} value={c.id}>{c.name}</option>
                ))}
              </select>
            </div>
          )}

          {scopeKind === 'cidr' && (
            <div className="form-group">
              <label htmlFor="sd-cidr">CIDR</label>
              <input
                id="sd-cidr"
                type="text"
                placeholder="10.0.0.0/16"
                value={cidr}
                onChange={(e) => setCidr(e.target.value)}
                required
              />
            </div>
          )}

          {kind === 'compliance' && (
            <div className="form-group">
              <label htmlFor="sd-bundle">Bundle</label>
              <select
                id="sd-bundle"
                value={bundleId}
                onChange={(e) => setBundleId(e.target.value)}
                required
              >
                <option value="">Select a bundle…</option>
                {bundles?.map((b) => (
                  <option key={b.id} value={b.id}>{b.name} v{b.version}</option>
                ))}
              </select>
            </div>
          )}

          <div className="form-group">
            <label htmlFor="sd-agent">Agent</label>
            <select
              id="sd-agent"
              value={agentId}
              onChange={(e) => setAgentId(e.target.value)}
            >
              <option value="">— unassigned —</option>
              {agents?.map((a) => (
                <option key={a.id} value={a.id}>{a.name} ({a.status})</option>
              ))}
            </select>
          </div>

          <div className="form-group">
            <label htmlFor="sd-schedule">Schedule (cron, optional)</label>
            <input
              id="sd-schedule"
              type="text"
              placeholder="e.g. 0 */6 * * *"
              value={schedule}
              onChange={(e) => setSchedule(e.target.value)}
            />
            <p className="muted" style={{ fontSize: 13, marginTop: 4 }}>
              {cronHelp(schedule)}
            </p>
          </div>

          <div className="form-group">
            <label style={{ fontWeight: 'normal' }}>
              <input
                type="checkbox"
                checked={enabled}
                onChange={(e) => setEnabled(e.target.checked)}
              />{' '}Enabled
            </label>
          </div>

          <button
            type="submit"
            className="btn btn-primary"
            disabled={createMut.isPending || updateMut.isPending}
          >
            {(createMut.isPending || updateMut.isPending)
              ? 'Saving…'
              : editingId ? 'Save changes' : 'Create definition'}
          </button>
          {(createMut.error || updateMut.error) && (
            <p className="error">
              {((createMut.error ?? updateMut.error) as Error).message}
            </p>
          )}
        </form>
      )}

      {isLoading && <p>Loading…</p>}
      {error && <p className="error">Failed to load: {(error as Error).message}</p>}
      {!isLoading && defs && defs.length === 0 && <p>No scan definitions yet.</p>}

      {defs && defs.length > 0 && (
        <table className="table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Type</th>
              <th>Scope</th>
              <th>Schedule</th>
              <th>Last</th>
              <th>Next</th>
              <th>Status</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {defs.map((d) => (
              <tr key={d.id} style={!d.enabled ? { opacity: 0.5 } : undefined}>
                <td>{d.name}</td>
                <td><span className="badge badge-type">{d.kind}</span></td>
                <td>{scopeLabel(d)}</td>
                <td>{d.schedule ?? <span className="muted">manual</span>}</td>
                <td>{d.last_run_at ? new Date(d.last_run_at).toLocaleString() : '—'}</td>
                <td>{d.next_run_at ? new Date(d.next_run_at).toLocaleString() : '—'}</td>
                <td><ScanStatusChip status={defStatus[d.id]} enabled={d.enabled} latestScan={latestScanForDef(d.id)} /></td>
                <td style={{ textAlign: 'right', whiteSpace: 'nowrap' }}>
                  <button
                    className="btn btn-sm"
                    style={{ marginRight: 6 }}
                    onClick={() => executeMut.mutate(d.id)}
                    disabled={executeMut.isPending}
                    title="Trigger a manual run now — does not shift the next scheduled run"
                  >
                    Run now
                  </button>
                  <button className="btn btn-sm" style={{ marginRight: 6 }} onClick={() => handleEdit(d)}>
                    Edit
                  </button>
                  <button
                    className="btn btn-sm"
                    style={{ marginRight: 6 }}
                    onClick={() => toggleMut.mutate({ id: d.id, enable: !d.enabled })}
                    disabled={toggleMut.isPending}
                  >
                    {d.enabled ? 'Disable' : 'Enable'}
                  </button>
                  <button
                    className="btn btn-sm btn-danger"
                    onClick={() => {
                      if (window.confirm(`Delete “${d.name}”?`)) deleteMut.mutate(d.id);
                    }}
                    disabled={deleteMut.isPending}
                  >
                    Delete
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {defs && defs.length > 0 && (
        <section style={{ marginTop: 24 }}>
          <h3>Coverage Impact</h3>
          <ul className="coverage-impact" style={{ paddingLeft: 18 }}>
            {defs.map((d, i) => {
              const q = coverageQueries[i];
              const count = q?.data?.endpoint_count;
              return (
                <li key={d.id}>
                  <strong>{d.name}</strong>{' '}
                  {q?.isLoading
                    ? <span className="muted">(loading coverage…)</span>
                    : q?.error
                      ? <span className="muted">(coverage unavailable)</span>
                      : count != null
                        ? <>covers <strong>{count}</strong> endpoint{count === 1 ? '' : 's'}</>
                        : <span className="muted">—</span>}
                </li>
              );
            })}
          </ul>
        </section>
      )}
    </div>
  );
}

// Unified status chip for a scan definition. Disabled takes precedence
// over the scan state so the row clearly communicates "not running".
function ScanStatusChip({ status, enabled, latestScan }: {
  status?: { status: string; queuedCount: number };
  enabled: boolean;
  latestScan?: Scan;
}) {
  if (!enabled) {
    return <span className="badge" style={{ background: '#9ca3af', color: '#fff' }}>Disabled</span>;
  }
  if (!status || status.status === 'idle') return null;
  switch (status.status) {
    case 'running':
      return (
        <span className="badge badge-completed" style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
          <span style={{
            width: 6, height: 6, borderRadius: '50%',
            background: 'var(--ss-success, #10b981)',
            animation: 'pulse 1.5s ease-in-out infinite',
          }} />
          Running
        </span>
      );
    case 'queued':
      return (
        <span className="badge badge-warning" style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
          Queued{status.queuedCount > 1 ? ` (${status.queuedCount})` : ''}
        </span>
      );
    case 'failed':
      return (
        <span
          className="badge badge-failed"
          title={latestScan?.error_message || undefined}
        >
          Failed
        </span>
      );
    default:
      return null;
  }
}
