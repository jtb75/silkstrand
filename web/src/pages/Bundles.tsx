import { useState, useRef } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useToast } from '../lib/toast';
import { listBundles, getBundleControls, uploadBundle } from '../api/client';
import type { Bundle, BundleControl } from '../api/types';

export default function Bundles() {
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [showUpload, setShowUpload] = useState(false);
  const [expandedId, setExpandedId] = useState<string | null>(null);

  const { data: bundles, isLoading, error } = useQuery<Bundle[]>({
    queryKey: ['bundles'],
    queryFn: listBundles,
  });

  return (
    <div>
      <div className="page-header" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <h2 style={{ margin: 0 }}>Bundles</h2>
        <button
          className="btn btn-primary"
          onClick={() => setShowUpload(!showUpload)}
        >
          {showUpload ? 'Cancel' : 'Upload bundle'}
        </button>
      </div>

      {showUpload && (
        <UploadModal
          onSuccess={() => {
            queryClient.invalidateQueries({ queryKey: ['bundles'] });
            setShowUpload(false);
            toast('Bundle uploaded', 'success');
          }}
          onCancel={() => setShowUpload(false)}
        />
      )}

      {isLoading && <p>Loading...</p>}
      {error && <p className="error">Failed to load bundles: {(error as Error).message}</p>}
      {!isLoading && bundles && bundles.length === 0 && (
        <p className="muted">No bundles registered. Upload a bundle to get started.</p>
      )}

      {bundles && bundles.length > 0 && (
        <table className="table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Version</th>
              <th>Engine</th>
              <th>Controls</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {bundles
              .filter((b) => b.id !== '11111111-1111-1111-1111-111111111111')
              .map((b) => (
              <BundleRow
                key={b.id}
                bundle={b}
                expanded={expandedId === b.id}
                onToggle={() => setExpandedId(expandedId === b.id ? null : b.id)}
              />
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}

function BundleRow({
  bundle,
  expanded,
  onToggle,
}: {
  bundle: Bundle;
  expanded: boolean;
  onToggle: () => void;
}) {
  const engine = bundle.engine ?? bundle.target_type ?? '—';
  const controlCount = bundle.control_count ?? 0;

  return (
    <>
      <tr>
        <td>{bundle.name}</td>
        <td>{bundle.version}</td>
        <td><span className="badge badge-type">{engine}</span></td>
        <td>{controlCount > 0 ? `${controlCount} controls` : '—'}</td>
        <td style={{ textAlign: 'right' }}>
          <button className="btn btn-sm" onClick={onToggle}>
            {expanded ? 'Hide controls' : 'View controls'}
          </button>
        </td>
      </tr>
      {expanded && (
        <tr>
          <td colSpan={5} style={{ padding: 0 }}>
            <ControlsPanel bundleId={bundle.id} />
          </td>
        </tr>
      )}
    </>
  );
}

function ControlsPanel({ bundleId }: { bundleId: string }) {
  const { data: controls, isLoading, error } = useQuery<BundleControl[]>({
    queryKey: ['bundle-controls', bundleId],
    queryFn: () => getBundleControls(bundleId),
  });

  if (isLoading) return <div style={{ padding: 16 }}>Loading controls...</div>;
  if (error) return <div style={{ padding: 16 }} className="error">Failed to load controls: {(error as Error).message}</div>;
  if (!controls || controls.length === 0) return <div style={{ padding: 16 }} className="muted">No controls registered for this bundle.</div>;

  return (
    <div style={{ padding: '8px 16px 16px' }}>
      <table className="table" style={{ marginBottom: 0 }}>
        <thead>
          <tr>
            <th>Control ID</th>
            <th>Name</th>
            <th>Severity</th>
            <th>Section</th>
            <th>Engine</th>
            <th>Versions</th>
            <th>Tags</th>
          </tr>
        </thead>
        <tbody>
          {controls.map((c) => {
            const versions = Array.isArray(c.engine_versions) ? c.engine_versions : [];
            const tags = Array.isArray(c.tags) ? c.tags : [];
            return (
              <tr key={c.control_id}>
                <td style={{ fontFamily: 'monospace', fontSize: 13 }}>{c.control_id}</td>
                <td>{c.name}</td>
                <td>
                  {c.severity
                    ? <SeverityBadge severity={c.severity} />
                    : <span className="muted">—</span>}
                </td>
                <td>{c.section ?? '—'}</td>
                <td>{c.engine}</td>
                <td>{versions.length > 0 ? versions.join(', ') : '—'}</td>
                <td>{tags.length > 0 ? tags.join(', ') : '—'}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function SeverityBadge({ severity }: { severity: string }) {
  const s = severity.toLowerCase();
  let cls = 'badge';
  if (s === 'critical' || s === 'high') cls += ' badge-failed';
  else if (s === 'medium') cls += ' badge-warning';
  else if (s === 'low' || s === 'info') cls += ' badge-completed';
  return <span className={cls}>{severity}</span>;
}

function UploadModal({
  onSuccess,
  onCancel,
}: {
  onSuccess: () => void;
  onCancel: () => void;
}) {
  const tarballRef = useRef<HTMLInputElement>(null);
  const signatureRef = useRef<HTMLInputElement>(null);
  const [uploading, setUploading] = useState(false);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);

  async function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setErrorMsg(null);
    const tarball = tarballRef.current?.files?.[0];
    if (!tarball) {
      setErrorMsg('Select a tarball file.');
      return;
    }
    const signature = signatureRef.current?.files?.[0];
    setUploading(true);
    try {
      await uploadBundle(tarball, signature);
      onSuccess();
    } catch (err) {
      setErrorMsg((err as Error).message);
    } finally {
      setUploading(false);
    }
  }

  return (
    <div className="form-card" style={{ maxWidth: 520, marginBottom: 24 }}>
      <h3 style={{ marginTop: 0 }}>Upload bundle</h3>
      <form onSubmit={handleSubmit}>
        <div className="form-group">
          <label htmlFor="bundle-tarball">Bundle tarball (.tar.gz)</label>
          <input
            id="bundle-tarball"
            type="file"
            accept=".tar.gz,.tgz"
            ref={tarballRef}
            required
          />
        </div>
        <div className="form-group">
          <label htmlFor="bundle-sig">Signature (.sig, optional)</label>
          <input
            id="bundle-sig"
            type="file"
            accept=".sig"
            ref={signatureRef}
          />
        </div>
        {errorMsg && <p className="error">{errorMsg}</p>}
        <div style={{ display: 'flex', gap: 8, marginTop: 12 }}>
          <button
            type="submit"
            className="btn btn-primary"
            disabled={uploading}
          >
            {uploading ? 'Uploading...' : 'Upload'}
          </button>
          <button
            type="button"
            className="btn"
            onClick={onCancel}
            disabled={uploading}
          >
            Cancel
          </button>
        </div>
      </form>
    </div>
  );
}
