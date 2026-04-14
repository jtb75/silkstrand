import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { listScans } from '../api/client';
import type { Scan } from '../api/types';

function StatusBadge({ status }: { status: string }) {
  return <span className={`badge badge-${status}`}>{status}</span>;
}

export default function Dashboard() {
  const { data: scans, isLoading, error } = useQuery<Scan[]>({
    queryKey: ['scans'],
    queryFn: listScans,
  });

  const recentScans = scans?.slice(0, 5) ?? [];

  return (
    <div>
      <h1>Dashboard</h1>
      <p>SilkStrand CIS Compliance Scanner</p>

      <div className="quick-links">
        <Link to="/targets" className="btn">
          Manage Targets
        </Link>
        <Link to="/scans" className="btn" style={{ marginLeft: 8 }}>
          View Scans
        </Link>
      </div>

      <h2 style={{ marginTop: 24 }}>Recent Scans</h2>
      {isLoading && <p>Loading...</p>}
      {error && <p className="error">Failed to load scans: {(error as Error).message}</p>}
      {!isLoading && recentScans.length === 0 && <p>No scans yet.</p>}
      {recentScans.length > 0 && (
        <table className="table">
          <thead>
            <tr>
              <th>Status</th>
              <th>Target</th>
              <th>Bundle</th>
              <th>Created</th>
            </tr>
          </thead>
          <tbody>
            {recentScans.map((scan) => (
              <tr key={scan.id}>
                <td>
                  <StatusBadge status={scan.status} />
                </td>
                <td>
                  <Link to={`/scans/${scan.id}`}>{(scan.target_id ?? scan.id).slice(0, 8)}...</Link>
                </td>
                <td>{scan.bundle_id}</td>
                <td>{new Date(scan.created_at).toLocaleString()}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
