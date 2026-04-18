import { useQuery, useMutation } from '@tanstack/react-query';
import { listFindings, getControlRego, copyTenantPolicy } from '../api/client';
import type { ControlEntry, Finding } from '../api/types';
import { formatAbsolute } from '../lib/time';
import { useToast } from '../lib/toast';

interface Props {
  control: ControlEntry;
  onClose: () => void;
}

function SeverityBadge({ severity }: { severity: string }) {
  const s = severity.toLowerCase();
  let cls = 'badge';
  if (s === 'critical' || s === 'high') cls += ' badge-failed';
  else if (s === 'medium') cls += ' badge-warning';
  else if (s === 'low' || s === 'info') cls += ' badge-completed';
  return <span className={cls}>{severity}</span>;
}

function FindingStatusBadge({ status }: { status: string }) {
  const cls =
    status === 'open'
      ? 'badge badge-failed'
      : status === 'suppressed'
        ? 'badge badge-warning'
        : 'badge badge-completed';
  return <span className={cls}>{status}</span>;
}

function assetLabel(f: Finding): string {
  const host = f.asset_hostname || f.asset_ip || f.asset_endpoint_id.slice(0, 8) + '...';
  return f.endpoint_port != null ? `${host}:${f.endpoint_port}` : host;
}

export default function ControlDetailDrawer({ control, onClose }: Props) {
  const { toast } = useToast();
  const versions = Array.isArray(control.engine_versions) ? control.engine_versions : [];
  const tags = Array.isArray(control.tags) ? control.tags : [];

  // Cross-framework findings: all open/suppressed findings whose source matches this control_id.
  const { data: findings, isLoading: findingsLoading } = useQuery<Finding[]>({
    queryKey: ['control-findings', control.control_id],
    queryFn: () => listFindings({ source: control.control_id, page_size: 100 }),
  });

  // Rego policy source (builtin).
  const { data: regoData, isLoading: regoLoading } = useQuery({
    queryKey: ['control-rego', control.control_id],
    queryFn: () => getControlRego(control.control_id),
  });

  const copyMut = useMutation({
    mutationFn: () => copyTenantPolicy(control.control_id),
    onSuccess: () => toast('Policy copied — edit in Compliance > Controls', 'success'),
    onError: (err: Error) => toast(err.message, 'error'),
  });

  return (
    <>
      <div className="drawer-backdrop" onClick={onClose} />
      <aside className="drawer">
        <header className="drawer-header">
          <h2 style={{ fontFamily: 'monospace', fontSize: 15 }}>{control.control_id}</h2>
          <button type="button" className="btn btn-sm" onClick={onClose}>
            x
          </button>
        </header>
        <div className="drawer-body">
          {/* Header */}
          <section>
            <h3>{control.name}</h3>
          </section>

          {/* Metadata */}
          <section>
            <h3>Metadata</h3>
            <div className="kv">
              <dt>Severity</dt>
              <dd><SeverityBadge severity={control.severity} /></dd>
              <dt>Engine</dt>
              <dd>{control.engine}</dd>
              <dt>Versions</dt>
              <dd>{versions.length > 0 ? versions.join(', ') : '\u2014'}</dd>
              <dt>Tags</dt>
              <dd>
                {tags.length > 0
                  ? tags.map((t) => (
                      <span
                        key={t}
                        className="badge"
                        style={{ fontSize: 11, padding: '1px 6px', marginRight: 4, opacity: 0.7 }}
                      >
                        {t}
                      </span>
                    ))
                  : '\u2014'}
              </dd>
            </div>
          </section>

          {/* Framework mappings */}
          <section>
            <h3>Framework mappings</h3>
            {control.frameworks.length === 0 ? (
              <p className="muted">No framework mappings.</p>
            ) : (
              <table className="table" style={{ fontSize: 13 }}>
                <thead>
                  <tr>
                    <th>Bundle</th>
                    <th>Section</th>
                  </tr>
                </thead>
                <tbody>
                  {control.frameworks.map((fw) => (
                    <tr key={`${fw.bundle_id}-${fw.section}`}>
                      <td>{fw.bundle_name}</td>
                      <td>{fw.section || '\u2014'}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </section>

          {/* Policy (Rego source) */}
          <section>
            <h3>Policy</h3>
            {regoLoading && <p>Loading policy...</p>}
            {!regoLoading && !regoData && (
              <p className="muted">No Rego policy on disk for this control.</p>
            )}
            {!regoLoading && regoData && (
              <>
                <pre style={{
                  background: '#1e293b',
                  color: '#e2e8f0',
                  padding: 16,
                  borderRadius: 6,
                  overflow: 'auto',
                  maxHeight: 400,
                  fontSize: 13,
                  lineHeight: 1.5,
                  fontFamily: 'monospace',
                }}>
                  {regoData.rego_source}
                </pre>
                <button
                  className="btn btn-sm"
                  style={{ marginTop: 8 }}
                  onClick={() => copyMut.mutate()}
                  disabled={copyMut.isPending}
                >
                  {copyMut.isPending ? 'Copying...' : 'Copy and Edit'}
                </button>
              </>
            )}
          </section>

          {/* Findings for this control */}
          <section>
            <h3>Findings</h3>
            {findingsLoading && <p>Loading findings...</p>}
            {!findingsLoading && (!findings || findings.length === 0) && (
              <p className="muted">No assets are failing this control.</p>
            )}
            {!findingsLoading && findings && findings.length > 0 && (
              <table className="table" style={{ fontSize: 13 }}>
                <thead>
                  <tr>
                    <th>Asset:Port</th>
                    <th>Status</th>
                    <th>Severity</th>
                    <th>Last seen</th>
                  </tr>
                </thead>
                <tbody>
                  {findings.map((f) => (
                    <tr key={f.id}>
                      <td style={{ fontFamily: 'monospace', fontSize: 12 }}>
                        {assetLabel(f)}
                      </td>
                      <td><FindingStatusBadge status={f.status} /></td>
                      <td>
                        {f.severity
                          ? <SeverityBadge severity={f.severity} />
                          : <span className="muted">{'\u2014'}</span>}
                      </td>
                      <td>{formatAbsolute(f.last_seen)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </section>
        </div>
      </aside>
    </>
  );
}
