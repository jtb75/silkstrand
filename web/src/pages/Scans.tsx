import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import {
  listScans, listTargets, createScan, listBundles, deleteScan,
  DISCOVERY_BUNDLE_ID,
} from '../api/client';
import type { Scan, Target, Bundle } from '../api/types';

// Target types eligible for a discovery scan (ADR 003 R1a recon pipeline).
const DISCOVERY_TARGET_TYPES = new Set(['host', 'cidr', 'network_range']);
type ScanKind = 'compliance' | 'discovery';

function StatusBadge({ status }: { status: string }) {
  return <span className={`badge badge-${status}`}>{status}</span>;
}

export default function Scans() {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [showForm, setShowForm] = useState(false);
  const [selectedTargetId, setSelectedTargetId] = useState('');
  const [scanKind, setScanKind] = useState<ScanKind>('compliance');

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
    mutationFn: ({ targetId, bundleId, kind }: { targetId: string; bundleId: string; kind: ScanKind }) =>
      createScan(targetId, bundleId, kind),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['scans'] });
      setShowForm(false);
      setSelectedTargetId('');
      setScanKind('compliance');
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteScan(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['scans'] });
    },
  });

  function handleDelete(e: React.MouseEvent, scan: Scan) {
    // Prevent the row-click navigation handler from firing.
    e.stopPropagation();
    if (scan.status === 'running') {
      alert("Can't delete a running scan. Wait for it to finish or be failed.");
      return;
    }
    if (!window.confirm(`Delete this ${scan.status} scan? Results will be removed.`)) {
      return;
    }
    deleteMutation.mutate(scan.id);
  }

  function handleCreate(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    const formData = new FormData(form);
    const targetId = formData.get('target_id') as string;
    const bundleId = scanKind === 'discovery'
      ? DISCOVERY_BUNDLE_ID
      : (formData.get('bundle_id') as string);
    createMutation.mutate({ targetId, bundleId, kind: scanKind });
  }

  return (
    <div>
      <div className="page-header">
        <h1>Scans</h1>
        <button
          className="btn btn-primary"
          onClick={() => {
            setShowForm(!showForm);
            setSelectedTargetId('');
            setScanKind('compliance');
          }}
        >
          {showForm ? 'Cancel' : 'New Scan'}
        </button>
      </div>

      {showForm && (
        <form className="form-card" onSubmit={handleCreate}>
          <div className="form-group">
            <label>Scan type</label>
            <div style={{ display: 'flex', gap: 16 }}>
              <label style={{ fontWeight: 'normal' }}>
                <input
                  type="radio"
                  name="scan_kind"
                  value="compliance"
                  checked={scanKind === 'compliance'}
                  onChange={() => { setScanKind('compliance'); setSelectedTargetId(''); }}
                />{' '}Compliance
              </label>
              <label style={{ fontWeight: 'normal' }}>
                <input
                  type="radio"
                  name="scan_kind"
                  value="discovery"
                  checked={scanKind === 'discovery'}
                  onChange={() => { setScanKind('discovery'); setSelectedTargetId(''); }}
                />{' '}Discovery
              </label>
            </div>
            {scanKind === 'discovery' && (
              <p className="muted" style={{ fontSize: 13, marginTop: 4 }}>
                Runs naabu → httpx → nuclei against the target. The agent's
                allowlist YAML gates everything — ranges outside the allowlist
                are rejected before any packet is sent.
              </p>
            )}
          </div>
          <div className="form-group">
            <label htmlFor="target_id">Target</label>
            <select
              id="target_id"
              name="target_id"
              required
              value={selectedTargetId}
              onChange={(e) => setSelectedTargetId(e.target.value)}
            >
              <option value="">
                {scanKind === 'discovery' ? 'Select a host / CIDR / range…' : 'Select a target…'}
              </option>
              {targets
                ?.filter((t) => scanKind === 'compliance' || DISCOVERY_TARGET_TYPES.has(t.type))
                .map((t) => (
                  <option key={t.id} value={t.id}>
                    {t.type}: {t.identifier}
                    {t.environment ? ` (${t.environment})` : ''}
                  </option>
                ))}
            </select>
            {scanKind === 'discovery' && targets && targets.filter((t) => DISCOVERY_TARGET_TYPES.has(t.type)).length === 0 && (
              <p className="muted" style={{ fontSize: 13, marginTop: 4 }}>
                No discovery-shaped targets yet. Create one with type{' '}
                <code>host</code>, <code>cidr</code>, or <code>network_range</code>.
              </p>
            )}
          </div>
          {scanKind === 'compliance' && (() => {
            const selectedTarget = targets?.find((t) => t.id === selectedTargetId);
            const compatibleBundles = selectedTarget
              ? bundles?.filter((b) => b.target_type === selectedTarget.type)
              : [];
            return (
              <div className="form-group">
                <label htmlFor="bundle_id">Bundle</label>
                <select
                  id="bundle_id"
                  name="bundle_id"
                  required
                  disabled={!selectedTarget}
                  key={selectedTargetId}
                >
                  <option value="">
                    {selectedTarget ? 'Select a bundle…' : 'Select a target first'}
                  </option>
                  {compatibleBundles?.map((b) => (
                    <option key={b.id} value={b.id}>
                      {b.name} v{b.version}
                    </option>
                  ))}
                </select>
                {selectedTarget && compatibleBundles?.length === 0 && (
                  <p className="muted" style={{ fontSize: 13, marginTop: 4 }}>
                    No bundles available for target type “{selectedTarget.type}”.
                  </p>
                )}
              </div>
            );
          })()}
          <button
            type="submit"
            className="btn btn-primary"
            disabled={createMutation.isPending}
          >
            {createMutation.isPending
              ? 'Creating…'
              : scanKind === 'discovery' ? 'Start discovery scan' : 'Start compliance scan'}
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
              <th></th>
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
                <td>{scan.target_id ? `${scan.target_id.slice(0, 8)}...` : '-'}</td>
                <td>{scan.bundle_id}</td>
                <td>{new Date(scan.created_at).toLocaleString()}</td>
                <td>
                  {scan.completed_at
                    ? new Date(scan.completed_at).toLocaleString()
                    : '-'}
                </td>
                <td>
                  <button
                    className="btn btn-small btn-danger"
                    disabled={scan.status === 'running' || deleteMutation.isPending}
                    title={scan.status === 'running'
                      ? 'Running scans cannot be deleted'
                      : 'Delete this scan and its results'}
                    onClick={(e) => handleDelete(e, scan)}
                  >
                    Delete
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
