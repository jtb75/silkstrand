import { useMemo, useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  listFindings,
  suppressFinding,
  reopenFinding,
  listAssetSets,
} from '../api/client';
import type {
  Finding,
  FindingSourceKind,
  FindingStatus,
  AssetSet,
} from '../api/types';

type Tab = 'vulnerabilities' | 'compliance';

const TABS: { value: Tab; label: string }[] = [
  { value: 'vulnerabilities', label: 'Vulnerabilities' },
  { value: 'compliance', label: 'Compliance' },
];

// SOC-facing top-level view. Vulnerabilities tab reads network_vuln;
// Compliance tab unions bundle_compliance + network_compliance so both
// categories land in one place.
const SOURCE_KINDS_BY_TAB: Record<Tab, FindingSourceKind[]> = {
  vulnerabilities: ['network_vuln'],
  compliance: ['bundle_compliance', 'network_compliance'],
};

function SeverityBadge({ severity }: { severity?: string }) {
  if (!severity) return <span className="muted">—</span>;
  return <span className={`badge badge-sev-${severity.toLowerCase()}`}>{severity}</span>;
}

function StatusBadge({ status }: { status: FindingStatus }) {
  return <span className={`badge badge-${status}`}>{status}</span>;
}

function assetLabel(f: Finding): string {
  const host = f.asset_hostname || f.asset_ip || f.asset_endpoint_id.slice(0, 8) + '…';
  return f.endpoint_port != null ? `${host}:${f.endpoint_port}` : host;
}

export default function Findings() {
  const queryClient = useQueryClient();
  const [tab, setTab] = useState<Tab>('vulnerabilities');

  const [severity, setSeverity] = useState('');
  const [status, setStatus] = useState<FindingStatus | ''>('open');
  const [collectionId, setCollectionId] = useState('');
  const [since, setSince] = useState('');
  const [until, setUntil] = useState('');

  const { data: assetSets } = useQuery<AssetSet[]>({
    queryKey: ['asset-sets'],
    queryFn: listAssetSets,
  });

  const params = useMemo(() => ({
    source_kind: SOURCE_KINDS_BY_TAB[tab],
    severity: severity || undefined,
    status: (status || undefined) as FindingStatus | undefined,
    collection_id: collectionId || undefined,
    since: since || undefined,
    until: until || undefined,
  }), [tab, severity, status, collectionId, since, until]);

  const { data: findings, isLoading, error } = useQuery<Finding[]>({
    queryKey: ['findings', params],
    queryFn: () => listFindings(params),
  });

  const suppressMut = useMutation({
    mutationFn: (id: string) => suppressFinding(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['findings'] }),
  });

  const reopenMut = useMutation({
    mutationFn: (id: string) => reopenFinding(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['findings'] }),
  });

  return (
    <div>
      <div className="page-header">
        <h1>Findings</h1>
      </div>

      <div className="tabbar" style={{ display: 'flex', gap: 16, borderBottom: '1px solid #e5e7eb', marginBottom: 16 }}>
        {TABS.map((t) => (
          <button
            key={t.value}
            onClick={() => setTab(t.value)}
            style={{
              background: 'none',
              border: 'none',
              padding: '8px 4px',
              cursor: 'pointer',
              fontWeight: tab === t.value ? 600 : 400,
              borderBottom: tab === t.value ? '2px solid #2563eb' : '2px solid transparent',
              marginBottom: -1,
            }}
          >
            {t.label}
          </button>
        ))}
      </div>

      <div className="form-card" style={{ display: 'flex', gap: 12, flexWrap: 'wrap' }}>
        <div>
          <label htmlFor="f-sev" style={{ display: 'block', fontSize: 12 }}>Severity</label>
          <select id="f-sev" value={severity} onChange={(e) => setSeverity(e.target.value)}>
            <option value="">All</option>
            <option value="critical">critical</option>
            <option value="high">high</option>
            <option value="medium">medium</option>
            <option value="low">low</option>
            <option value="info">info</option>
          </select>
        </div>
        <div>
          <label htmlFor="f-status" style={{ display: 'block', fontSize: 12 }}>Status</label>
          <select id="f-status" value={status} onChange={(e) => setStatus(e.target.value as FindingStatus | '')}>
            <option value="">All</option>
            <option value="open">open</option>
            <option value="resolved">resolved</option>
            <option value="suppressed">suppressed</option>
          </select>
        </div>
        <div>
          <label htmlFor="f-coll" style={{ display: 'block', fontSize: 12 }}>Collection</label>
          <select id="f-coll" value={collectionId} onChange={(e) => setCollectionId(e.target.value)}>
            <option value="">All</option>
            {assetSets?.map((a) => (
              <option key={a.id} value={a.id}>{a.name}</option>
            ))}
          </select>
        </div>
        <div>
          <label htmlFor="f-since" style={{ display: 'block', fontSize: 12 }}>Since</label>
          <input id="f-since" type="datetime-local" value={since} onChange={(e) => setSince(e.target.value)} />
        </div>
        <div>
          <label htmlFor="f-until" style={{ display: 'block', fontSize: 12 }}>Until</label>
          <input id="f-until" type="datetime-local" value={until} onChange={(e) => setUntil(e.target.value)} />
        </div>
      </div>

      {isLoading && <p>Loading…</p>}
      {error && <p className="error">Failed to load: {(error as Error).message}</p>}
      {!isLoading && findings && findings.length === 0 && <p>No findings match.</p>}

      {findings && findings.length > 0 && (
        <table className="table">
          <thead>
            <tr>
              <th>Severity</th>
              <th>Title</th>
              <th>Source</th>
              <th>Asset:Port</th>
              <th>Status</th>
              <th>Last Seen</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {findings.map((f) => (
              <tr key={f.id}>
                <td><SeverityBadge severity={f.severity} /></td>
                <td>{f.title}</td>
                <td title={f.source_kind}>{f.source}{f.source_id ? ` / ${f.source_id}` : ''}</td>
                <td>{assetLabel(f)}</td>
                <td><StatusBadge status={f.status} /></td>
                <td>{new Date(f.last_seen).toLocaleString()}</td>
                <td style={{ textAlign: 'right', whiteSpace: 'nowrap' }}>
                  {f.status === 'open' && (
                    <button
                      className="btn btn-sm"
                      onClick={() => suppressMut.mutate(f.id)}
                      disabled={suppressMut.isPending}
                    >
                      Suppress
                    </button>
                  )}
                  {f.status === 'suppressed' && (
                    <button
                      className="btn btn-sm"
                      onClick={() => reopenMut.mutate(f.id)}
                      disabled={reopenMut.isPending}
                    >
                      Reopen
                    </button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
