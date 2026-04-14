import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  listOneShotScans,
  listAssetSets,
  listAgents,
  listBundles,
  createOneShotScan,
  type CreateOneShotScanRequest,
} from '../api/client';

// D13 one-shot scans admin page. List historical fan-outs + launch a
// new one picking an asset set, bundle, and agent.
export default function OneShotScans() {
  const queryClient = useQueryClient();
  const { data: scans, isLoading, error } = useQuery({
    queryKey: ['one-shot-scans'],
    queryFn: listOneShotScans,
    // One-shots complete asynchronously; poll while any are running.
    refetchInterval: (query) => {
      const d = query.state.data as ReturnType<typeof listOneShotScans> extends Promise<infer T> ? T : never;
      if (d?.some((o) => o.status === 'running' || o.status === 'pending')) return 5000;
      return false;
    },
  });

  const [showForm, setShowForm] = useState(false);
  const createMut = useMutation({
    mutationFn: createOneShotScan,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['one-shot-scans'] });
      setShowForm(false);
    },
  });

  return (
    <div>
      <div className="page-header">
        <h1>One-shot Scans</h1>
        <button className="btn btn-primary" onClick={() => setShowForm(!showForm)}>
          {showForm ? 'Cancel' : 'New One-shot'}
        </button>
      </div>

      {showForm && (
        <OneShotForm
          submitting={createMut.isPending}
          error={createMut.error ? (createMut.error as Error).message : null}
          onSubmit={(req) => createMut.mutate(req)}
        />
      )}

      {isLoading && <p>Loading…</p>}
      {error && <p className="error">{(error as Error).message}</p>}
      {!isLoading && scans && scans.length === 0 && (
        <p className="muted">No one-shot scans yet.</p>
      )}
      {scans && scans.length > 0 && (
        <table className="table">
          <thead>
            <tr>
              <th>ID</th>
              <th>Status</th>
              <th>Bundle</th>
              <th>Targets</th>
              <th>Triggered by</th>
              <th>Created</th>
            </tr>
          </thead>
          <tbody>
            {scans.map((o) => (
              <tr key={o.id}>
                <td>{o.id.slice(0, 8)}…</td>
                <td><span className={`badge badge-${o.status}`}>{o.status}</span></td>
                <td>{o.bundle_id.slice(0, 8)}…</td>
                <td>{o.completed_targets}/{o.total_targets ?? '?'}</td>
                <td>{o.triggered_by || '-'}</td>
                <td>{new Date(o.created_at).toLocaleString()}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}

interface FormProps {
  submitting: boolean;
  error: string | null;
  onSubmit: (req: CreateOneShotScanRequest) => void;
}

function OneShotForm({ submitting, error, onSubmit }: FormProps) {
  const { data: sets } = useQuery({ queryKey: ['asset-sets'], queryFn: listAssetSets });
  const { data: agents } = useQuery({ queryKey: ['agents'], queryFn: listAgents });
  const { data: bundles } = useQuery({ queryKey: ['bundles'], queryFn: listBundles });

  function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const fd = new FormData(e.currentTarget);
    const req: CreateOneShotScanRequest = {
      bundle_id: fd.get('bundle_id') as string,
      agent_id: fd.get('agent_id') as string,
      asset_set_id: fd.get('asset_set_id') as string,
    };
    onSubmit(req);
  }

  return (
    <form className="form-card" onSubmit={handleSubmit}>
      <div className="form-group">
        <label htmlFor="asset_set_id">Asset Set</label>
        <select id="asset_set_id" name="asset_set_id" required>
          <option value="">Select an asset set…</option>
          {sets?.map((s) => (
            <option key={s.id} value={s.id}>{s.name}</option>
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
      </div>
      <div className="form-group">
        <label htmlFor="agent_id">Agent</label>
        <select id="agent_id" name="agent_id" required>
          <option value="">Select an agent…</option>
          {agents?.map((a) => (
            <option key={a.id} value={a.id}>
              {a.name} ({a.status})
            </option>
          ))}
        </select>
      </div>
      <button type="submit" className="btn btn-primary" disabled={submitting}>
        {submitting ? 'Dispatching…' : 'Dispatch'}
      </button>
      {error && <p className="error">{error}</p>}
    </form>
  );
}
