import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import { listScans, listTargets, createScan, listBundles } from '../api/client';
import type { Scan, Target, Bundle } from '../api/types';

function StatusBadge({ status }: { status: string }) {
  return <span className={`badge badge-${status}`}>{status}</span>;
}

export default function Scans() {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [showForm, setShowForm] = useState(false);

  const { data: scans, isLoading, error } = useQuery<Scan[]>({
    queryKey: ['scans'],
    queryFn: listScans,
    refetchInterval: (query) => {
      const data = query.state.data as Scan[] | undefined;
      if (data?.some((s) => s.status === 'pending' || s.status === 'running')) {
        return 5000;
      }
      return false;
    },
  });

  const { data: targets } = useQuery<Target[]>({
    queryKey: ['targets'],
    queryFn: listTargets,
    enabled: showForm,
  });

  const { data: bundles } = useQuery<Bundle[]>({
    queryKey: ['bundles'],
    queryFn: listBundles,
    enabled: showForm,
  });

  const createMutation = useMutation({
    mutationFn: ({ targetId, bundleId }: { targetId: string; bundleId: string }) =>
      createScan(targetId, bundleId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['scans'] });
      setShowForm(false);
    },
  });

  function handleCreate(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    const formData = new FormData(form);
    createMutation.mutate({
      targetId: formData.get('target_id') as string,
      bundleId: formData.get('bundle_id') as string,
    });
  }

  return (
    <div>
      <div className="page-header">
        <h1>Scans</h1>
        <button className="btn btn-primary" onClick={() => setShowForm(!showForm)}>
          {showForm ? 'Cancel' : 'New Scan'}
        </button>
      </div>

      {showForm && (
        <form className="form-card" onSubmit={handleCreate}>
          <div className="form-group">
            <label htmlFor="target_id">Target</label>
            <select id="target_id" name="target_id" required>
              <option value="">Select a target...</option>
              {targets?.map((t) => (
                <option key={t.id} value={t.id}>
                  {t.type}: {t.identifier}
                  {t.environment ? ` (${t.environment})` : ''}
                </option>
              ))}
            </select>
          </div>
          <div className="form-group">
            <label htmlFor="bundle_id">Bundle</label>
            <select id="bundle_id" name="bundle_id" required>
              <option value="">Select a bundle…</option>
              {bundles?.map((b) => (
                <option key={b.id} value={b.id}>
                  {b.name} v{b.version} ({b.target_type})
                </option>
              ))}
            </select>
            {bundles && bundles.length === 0 && (
              <p className="muted" style={{ fontSize: 13, marginTop: 4 }}>
                No bundles available. Contact your SilkStrand administrator.
              </p>
            )}
          </div>
          <button
            type="submit"
            className="btn btn-primary"
            disabled={createMutation.isPending}
          >
            {createMutation.isPending ? 'Creating...' : 'Start Scan'}
          </button>
          {createMutation.error && (
            <p className="error">{(createMutation.error as Error).message}</p>
          )}
        </form>
      )}

      {isLoading && <p>Loading...</p>}
      {error && <p className="error">Failed to load scans: {(error as Error).message}</p>}
      {!isLoading && scans && scans.length === 0 && <p>No scans yet.</p>}
      {scans && scans.length > 0 && (
        <table className="table">
          <thead>
            <tr>
              <th>Status</th>
              <th>Target</th>
              <th>Bundle</th>
              <th>Created</th>
              <th>Completed</th>
            </tr>
          </thead>
          <tbody>
            {scans.map((scan) => (
              <tr
                key={scan.id}
                className="clickable-row"
                onClick={() => navigate(`/scans/${scan.id}`)}
              >
                <td>
                  <StatusBadge status={scan.status} />
                </td>
                <td>{scan.target_id.slice(0, 8)}...</td>
                <td>{scan.bundle_id}</td>
                <td>{new Date(scan.created_at).toLocaleString()}</td>
                <td>
                  {scan.completed_at
                    ? new Date(scan.completed_at).toLocaleString()
                    : '-'}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
