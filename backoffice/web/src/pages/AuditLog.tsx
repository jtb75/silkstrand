import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { listAuditLog } from '../api/client';
import type { AuditEntry } from '../api/types';

/**
 * Audit Log — cross-tenant view of who did what. Backoffice admin only.
 * Simple filter bar; no pagination yet (store caps at 500 rows).
 */
export default function AuditLog() {
  const [action, setAction] = useState('');
  const [actorId, setActorId] = useState('');
  const [tenantId, setTenantId] = useState('');

  const { data, isLoading, error, refetch } = useQuery<AuditEntry[]>({
    queryKey: ['audit', { action, actorId, tenantId }],
    queryFn: () => listAuditLog({
      action: action || undefined,
      actor_id: actorId || undefined,
      tenant_id: tenantId || undefined,
      limit: 200,
    }),
  });

  return (
    <div>
      <div className="page-header">
        <h1>Audit Log</h1>
        <button className="btn" onClick={() => refetch()}>Refresh</button>
      </div>

      <div className="filter-bar" style={{ gap: 8 }}>
        <input
          placeholder="Filter by action (e.g. tenant.create)"
          value={action} onChange={(e) => setAction(e.target.value)}
          style={{ flex: 1 }}
        />
        <input
          placeholder="Actor ID"
          value={actorId} onChange={(e) => setActorId(e.target.value)}
          style={{ flex: 1 }}
        />
        <input
          placeholder="Tenant ID"
          value={tenantId} onChange={(e) => setTenantId(e.target.value)}
          style={{ flex: 1 }}
        />
      </div>

      {isLoading && <p>Loading…</p>}
      {error && <p className="error">{(error as Error).message}</p>}
      {!isLoading && data && data.length === 0 && <p>No entries match.</p>}
      {data && data.length > 0 && (
        <table className="table">
          <thead>
            <tr>
              <th>When</th>
              <th>Actor</th>
              <th>Action</th>
              <th>Target</th>
              <th>Tenant</th>
              <th>Details</th>
            </tr>
          </thead>
          <tbody>
            {data.map((e) => (
              <tr key={e.id}>
                <td style={{ whiteSpace: 'nowrap' }}>
                  {new Date(e.occurred_at).toLocaleString()}
                </td>
                <td>
                  <div>{e.actor_email || '—'}</div>
                  <div className="text-muted" style={{ fontSize: 12 }}>{e.actor_type}</div>
                </td>
                <td><code>{e.action}</code></td>
                <td className="text-muted" style={{ fontSize: 12 }}>
                  {e.target_type && e.target_id ? `${e.target_type}:${e.target_id.slice(0, 8)}` : '—'}
                </td>
                <td className="text-muted" style={{ fontSize: 12 }}>
                  {e.tenant_id ? e.tenant_id.slice(0, 8) : '—'}
                </td>
                <td style={{ fontSize: 12 }}>
                  {e.metadata && Object.keys(e.metadata).length > 0
                    ? JSON.stringify(e.metadata)
                    : ''}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
