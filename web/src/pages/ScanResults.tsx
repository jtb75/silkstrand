import { useState } from 'react';
import { useParams, Link } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { getScan, listFindings, getScanFacts, replayEvaluation } from '../api/client';
import type { CollectedFactsEntry } from '../api/client';
import type { Scan, ScanResult, Finding } from '../api/types';
import AgentLogConsole from '../components/AgentLogConsole';

type ResultsTab = 'overview' | 'findings' | 'facts' | 'console';

function StatusBadge({ status }: { status: string }) {
  return <span className={`badge badge-${status}`}>{status}</span>;
}

function ControlStatusBadge({ status }: { status: string }) {
  return <span className={`badge badge-control-${status}`}>{status}</span>;
}

function SummaryBar({ scan }: { scan: Scan }) {
  const summary = scan.summary;
  if (!summary || summary.total === 0) return null;

  const segments = [
    { label: 'Pass', count: summary.pass, className: 'segment-pass' },
    { label: 'Fail', count: summary.fail, className: 'segment-fail' },
    { label: 'Error', count: summary.error, className: 'segment-error' },
    { label: 'N/A', count: summary.not_applicable, className: 'segment-na' },
  ];

  return (
    <div className="summary-section">
      <div className="summary-bar">
        {segments.map((seg) =>
          seg.count > 0 ? (
            <div
              key={seg.label}
              className={`summary-segment ${seg.className}`}
              style={{ flex: seg.count }}
              title={`${seg.label}: ${seg.count}`}
            />
          ) : null,
        )}
      </div>
      <div className="summary-labels">
        {segments.map((seg) => (
          <span key={seg.label} className="summary-label">
            {seg.label}: {seg.count}
          </span>
        ))}
        <span className="summary-label">Total: {summary.total}</span>
      </div>
    </div>
  );
}

function ResultRow({ result }: { result: ScanResult }) {
  const [expanded, setExpanded] = useState(false);

  return (
    <>
      <tr className="clickable-row" onClick={() => setExpanded(!expanded)}>
        <td>{result.control_id}</td>
        <td>{result.title}</td>
        <td>
          <ControlStatusBadge status={result.status} />
        </td>
        <td>{result.severity || '-'}</td>
        <td className="expand-indicator">{expanded ? '-' : '+'}</td>
      </tr>
      {expanded && (
        <tr className="expanded-row">
          <td colSpan={5}>
            {result.evidence && (
              <div className="result-detail">
                <strong>Evidence:</strong>
                <pre>{JSON.stringify(result.evidence, null, 2)}</pre>
              </div>
            )}
            {result.remediation && (
              <div className="result-detail">
                <strong>Remediation:</strong>
                <p>{result.remediation}</p>
              </div>
            )}
            {!result.evidence && !result.remediation && (
              <p className="text-muted">No additional details.</p>
            )}
          </td>
        </tr>
      )}
    </>
  );
}

function FindingsTab({ scanId }: { scanId: string }) {
  const { data, isLoading, error } = useQuery<Finding[]>({
    queryKey: ['findings', { scan_id: scanId }],
    queryFn: () => listFindings({ scan_id: scanId }),
  });

  if (isLoading) return <p>Loading findings…</p>;
  if (error) return <p className="error">Failed to load findings: {(error as Error).message}</p>;
  if (!data || data.length === 0) return <p>No findings for this scan.</p>;

  return (
    <table className="table">
      <thead>
        <tr>
          <th>Severity</th>
          <th>Title</th>
          <th>Source</th>
          <th>Endpoint</th>
          <th>Status</th>
          <th>Last Seen</th>
        </tr>
      </thead>
      <tbody>
        {data.map((f) => (
          <tr key={f.id}>
            <td><span className={`badge badge-sev-${(f.severity ?? '').toLowerCase()}`}>{f.severity ?? '—'}</span></td>
            <td>{f.title}</td>
            <td title={f.source_kind}>{f.source}{f.source_id ? ` / ${f.source_id}` : ''}</td>
            <td>
              {(f.asset_hostname || f.asset_ip || f.asset_endpoint_id.slice(0, 8) + '…')}
              {f.endpoint_port != null ? `:${f.endpoint_port}` : ''}
            </td>
            <td><span className={`badge badge-${f.status}`}>{f.status}</span></td>
            <td>{new Date(f.last_seen).toLocaleString()}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

export default function ScanResults() {
  const { id } = useParams<{ id: string }>();
  const [tab, setTab] = useState<ResultsTab>('overview');

  const { data: scan, isLoading, error } = useQuery<Scan>({
    queryKey: ['scan', id],
    queryFn: () => getScan(id!),
    enabled: !!id,
    refetchInterval: (query) => {
      const data = query.state.data as Scan | undefined;
      if (data?.status === 'running' || data?.status === 'pending' || data?.status === 'queued') {
        return 5000;
      }
      return false;
    },
  });

  if (isLoading) return <p>Loading...</p>;
  if (error) return <p className="error">Failed to load scan: {(error as Error).message}</p>;
  if (!scan) return <p>Scan not found.</p>;

  return (
    <div>
      <Link to="/scans" className="back-link">
        Back to Scans
      </Link>

      <h1>Scan Results</h1>

      <div className="tabbar" style={{ display: 'flex', gap: 16, borderBottom: '1px solid #e5e7eb', marginBottom: 16 }}>
        {(['overview', 'findings', 'facts', 'console'] as ResultsTab[]).map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            style={{
              background: 'none',
              border: 'none',
              padding: '8px 4px',
              cursor: 'pointer',
              fontWeight: tab === t ? 600 : 400,
              borderBottom: tab === t ? '2px solid #2563eb' : '2px solid transparent',
              marginBottom: -1,
              textTransform: 'capitalize',
            }}
          >
            {t}
          </button>
        ))}
      </div>

      {tab === 'findings' && id && <FindingsTab scanId={id} />}
      {tab === 'overview' && <ScanOverview scan={scan} /> }
      {tab === 'facts' && id && <FactsTab scanId={id} />}
      {tab === 'console' && id && (
        <div className="scan-console-wrap">
          <AgentLogConsole filter={{ scanId: id }} />
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Facts tab — collected facts from ADR 011 collector pipeline
// ---------------------------------------------------------------------------

function FactsTab({ scanId }: { scanId: string }) {
  const { data, isLoading, error } = useQuery<{ items: CollectedFactsEntry[] }>({
    queryKey: ['scan-facts', scanId],
    queryFn: () => getScanFacts(scanId),
  });

  if (isLoading) return <p>Loading facts...</p>;
  if (error) return <p className="error">Failed to load facts: {(error as Error).message}</p>;

  const items = data?.items ?? [];

  if (items.length === 0) {
    return (
      <div className="detail-card" style={{ marginTop: 12 }}>
        <p className="muted">
          No facts collected — this scan used the legacy bundle runner.
        </p>
      </div>
    );
  }

  return (
    <div>
      {items.map((entry) => (
        <CollectorFactsSection key={entry.collector_id} entry={entry} />
      ))}
    </div>
  );
}

function CollectorFactsSection({ entry }: { entry: CollectedFactsEntry }) {
  const [collapsed, setCollapsed] = useState(false);
  return (
    <div className="detail-card" style={{ marginBottom: 16 }}>
      <div
        style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', cursor: 'pointer' }}
        onClick={() => setCollapsed(!collapsed)}
      >
        <div>
          <strong style={{ fontFamily: 'monospace', fontSize: 14 }}>{entry.collector_id}</strong>
          <span className="muted" style={{ marginLeft: 12, fontSize: 12 }}>
            {new Date(entry.collected_at).toLocaleString()}
          </span>
        </div>
        <span style={{ fontWeight: 600, fontSize: 16 }}>{collapsed ? '+' : '\u2212'}</span>
      </div>
      {!collapsed && (
        <pre style={{
          background: '#1e293b',
          color: '#e2e8f0',
          padding: 16,
          borderRadius: 6,
          marginTop: 12,
          marginBottom: 0,
          overflow: 'auto',
          maxHeight: 500,
          fontSize: 13,
          lineHeight: 1.5,
        }}>
          {JSON.stringify(entry.facts, null, 2)}
        </pre>
      )}
    </div>
  );
}

function ReEvaluateButton({ scanId }: { scanId: string }) {
  const queryClient = useQueryClient();
  const [toast, setToast] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: () => replayEvaluation({ scan_id: scanId }),
    onSuccess: (data) => {
      setToast(
        `Re-evaluation complete: ${data.findings_created} created, ${data.findings_updated} updated`,
      );
      void queryClient.invalidateQueries({ queryKey: ['findings'] });
      void queryClient.invalidateQueries({ queryKey: ['scan', scanId] });
      setTimeout(() => setToast(null), 5000);
    },
    onError: (err: Error) => {
      setToast(`Re-evaluation failed: ${err.message}`);
      setTimeout(() => setToast(null), 5000);
    },
  });

  return (
    <div style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
      <button
        className="btn btn-secondary"
        disabled={mutation.isPending}
        onClick={() => mutation.mutate()}
        title="Re-evaluate stored facts against current policies"
      >
        {mutation.isPending ? 'Re-evaluating...' : 'Re-evaluate'}
      </button>
      {toast && (
        <span
          style={{
            padding: '4px 10px',
            borderRadius: 4,
            fontSize: 13,
            background: mutation.isError ? '#fef2f2' : '#f0fdf4',
            color: mutation.isError ? '#991b1b' : '#166534',
          }}
        >
          {toast}
        </span>
      )}
    </div>
  );
}

function ScanOverview({ scan }: { scan: Scan }) {
  return (
    <div>

      <div className="scan-meta">
        <div>
          <strong>Status:</strong> <StatusBadge status={scan.status} />
        </div>
        <div>
          <strong>Target:</strong> {scan.target_id}
        </div>
        <div>
          <strong>Bundle:</strong> {scan.bundle_id}
        </div>
        <div>
          <strong>Created:</strong> {new Date(scan.created_at).toLocaleString()}
        </div>
        {scan.started_at && (
          <div>
            <strong>Started:</strong> {new Date(scan.started_at).toLocaleString()}
          </div>
        )}
        {scan.completed_at && (
          <div>
            <strong>Completed:</strong> {new Date(scan.completed_at).toLocaleString()}
          </div>
        )}
      </div>

      {scan.status === 'failed' && scan.error_message && (
        <div className="detail-card" style={{ borderColor: '#b91c1c', marginTop: 12 }}>
          <strong>Failure reason:</strong>
          <pre style={{ whiteSpace: 'pre-wrap', marginTop: 6, marginBottom: 0 }}>
            {scan.error_message}
          </pre>
        </div>
      )}

      <SummaryBar scan={scan} />

      {(scan.status === 'completed' || scan.status === 'failed') && (
        <div style={{ margin: '12px 0' }}>
          <ReEvaluateButton scanId={scan.id} />
        </div>
      )}

      {scan.results && scan.results.length > 0 ? (
        <table className="table">
          <thead>
            <tr>
              <th>Control ID</th>
              <th>Title</th>
              <th>Status</th>
              <th>Severity</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {scan.results.map((r) => (
              <ResultRow key={r.id} result={r} />
            ))}
          </tbody>
        </table>
      ) : (
        <p>
          {scan.status === 'queued'
            ? 'Scan queued — waiting for agent...'
            : scan.status === 'pending' || scan.status === 'running'
              ? 'Scan in progress...'
              : 'No results.'}
        </p>
      )}
    </div>
  );
}
