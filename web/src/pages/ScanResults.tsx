import { useState } from 'react';
import { useParams, Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { getScan } from '../api/client';
import type { Scan, ScanResult } from '../api/types';

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

export default function ScanResults() {
  const { id } = useParams<{ id: string }>();

  const { data: scan, isLoading, error } = useQuery<Scan>({
    queryKey: ['scan', id],
    queryFn: () => getScan(id!),
    enabled: !!id,
    refetchInterval: (query) => {
      const data = query.state.data as Scan | undefined;
      if (data?.status === 'running' || data?.status === 'pending') {
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
          {scan.status === 'pending' || scan.status === 'running'
            ? 'Scan in progress...'
            : 'No results.'}
        </p>
      )}
    </div>
  );
}
