import { useState, useRef, useMemo } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useSearchParams } from 'react-router-dom';
import { useToast } from '../lib/toast';
import { listBundles, getBundleControls, uploadBundle, listControls } from '../api/client';
import type { Bundle, BundleControl, ControlEntry } from '../api/types';
import ControlDetailDrawer from '../components/ControlDetailDrawer';
import FrameworkChip from '../components/FrameworkChip';

type Tab = 'frameworks' | 'controls' | 'profiles';

export default function Compliance() {
  const [searchParams, setSearchParams] = useSearchParams();
  const tab = (searchParams.get('tab') as Tab) || 'frameworks';

  function setTab(t: Tab) {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev);
      next.set('tab', t);
      return next;
    });
  }

  return (
    <div>
      <h1>Compliance</h1>

      <div className="tab-bar" style={{ display: 'flex', gap: 4, borderBottom: '1px solid #e5e7eb', marginTop: 16 }}>
        <TabButton active={tab === 'frameworks'} onClick={() => setTab('frameworks')}>Frameworks</TabButton>
        <TabButton active={tab === 'controls'} onClick={() => setTab('controls')}>Controls</TabButton>
        <TabButton active={tab === 'profiles'} onClick={() => setTab('profiles')}>Profiles</TabButton>
      </div>

      <div style={{ marginTop: 24 }}>
        {tab === 'frameworks' && <FrameworksTab />}
        {tab === 'controls' && <ControlsTab />}
        {tab === 'profiles' && <ProfilesPlaceholder />}
      </div>
    </div>
  );
}

function TabButton({ active, children, onClick }: { active: boolean; children: React.ReactNode; onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      style={{
        padding: '8px 16px',
        border: 'none',
        borderBottom: active ? '2px solid #0f766e' : '2px solid transparent',
        background: 'none',
        fontWeight: active ? 600 : 400,
        cursor: 'pointer',
      }}
    >
      {children}
    </button>
  );
}

function FrameworksTab() {
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
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
        <h2 style={{ margin: 0 }}>Frameworks</h2>
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
              <th>Hash</th>
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
  const engine = bundle.engine ?? bundle.target_type ?? '\u2014';
  const controlCount = bundle.control_count ?? 0;
  const hash = bundle.tarball_hash;
  const truncatedHash = hash ? hash.substring(0, 12) + '\u2026' : '\u2014';

  return (
    <>
      <tr>
        <td>{bundle.name}</td>
        <td>{bundle.version}</td>
        <td><span className="badge badge-type">{engine}</span></td>
        <td>{controlCount > 0 ? `${controlCount} controls` : '\u2014'}</td>
        <td>
          {hash ? (
            <span
              title={hash}
              style={{ fontFamily: 'monospace', fontSize: 12, cursor: 'help' }}
            >
              {truncatedHash}
            </span>
          ) : (
            <span className="muted">{'\u2014'}</span>
          )}
        </td>
        <td style={{ textAlign: 'right' }}>
          <button className="btn btn-sm" onClick={onToggle}>
            {expanded ? 'Hide controls' : 'View controls'}
          </button>
        </td>
      </tr>
      {expanded && (
        <tr>
          <td colSpan={6} style={{ padding: 0 }}>
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
                    : <span className="muted">{'\u2014'}</span>}
                </td>
                <td>{c.section ?? '\u2014'}</td>
                <td>{c.engine}</td>
                <td>{versions.length > 0 ? versions.join(', ') : '\u2014'}</td>
                <td>{tags.length > 0 ? tags.join(', ') : '\u2014'}</td>
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

// ---------------------------------------------------------------------------
// Controls tab — cross-framework control browser (Level 2B)
// ---------------------------------------------------------------------------

const SEVERITY_OPTIONS = ['critical', 'high', 'medium', 'low', 'info'] as const;

const ENGINE_OPTIONS = [
  'postgresql',
  'mssql',
  'mongodb',
  'mysql',
  'host',
  'cidr',
  'cloud',
] as const;

function ControlsTab() {
  const [searchParams, setSearchParams] = useSearchParams();

  // Read filter state from URL search params.
  const framework = searchParams.get('framework') || '';
  const engine = searchParams.get('engine') || '';
  const severity = searchParams.get('severity') || '';
  const tagFilter = searchParams.get('tag') || '';
  const searchText = searchParams.get('q') || '';

  const [selectedControl, setSelectedControl] = useState<ControlEntry | null>(null);

  function setFilter(key: string, value: string) {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev);
      if (value) next.set(key, value);
      else next.delete(key);
      // Ensure tab stays
      if (!next.has('tab')) next.set('tab', 'controls');
      return next;
    });
  }

  // Fetch bundles to populate the framework dropdown options.
  const { data: bundles } = useQuery<Bundle[]>({
    queryKey: ['bundles'],
    queryFn: listBundles,
  });

  const frameworkOptions = useMemo(() => {
    if (!bundles) return [];
    const names = new Set<string>();
    for (const b of bundles) {
      if (b.id !== '11111111-1111-1111-1111-111111111111' && b.framework) {
        names.add(b.framework);
      }
    }
    return Array.from(names).sort();
  }, [bundles]);

  // Fetch controls with applied filters.
  const { data, isLoading, error } = useQuery({
    queryKey: ['controls', { framework, engine, severity, tag: tagFilter, q: searchText }],
    queryFn: () =>
      listControls({
        framework: framework || undefined,
        engine: engine || undefined,
        severity: severity || undefined,
        tag: tagFilter || undefined,
        q: searchText || undefined,
      }),
  });

  const controls = data?.items ?? [];

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
        <h2 style={{ margin: 0 }}>Controls</h2>
        {data && <span className="muted">{data.total} control{data.total !== 1 ? 's' : ''}</span>}
      </div>

      {/* Filter bar */}
      <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginBottom: 16 }}>
        <select
          value={framework}
          onChange={(e) => setFilter('framework', e.target.value)}
          style={{ minWidth: 140 }}
        >
          <option value="">All frameworks</option>
          {frameworkOptions.map((f) => (
            <option key={f} value={f}>{f}</option>
          ))}
        </select>

        <select
          value={engine}
          onChange={(e) => setFilter('engine', e.target.value)}
          style={{ minWidth: 120 }}
        >
          <option value="">All engines</option>
          {ENGINE_OPTIONS.map((e) => (
            <option key={e} value={e}>{e}</option>
          ))}
        </select>

        <select
          value={severity}
          onChange={(e) => setFilter('severity', e.target.value)}
          style={{ minWidth: 110 }}
        >
          <option value="">All severities</option>
          {SEVERITY_OPTIONS.map((s) => (
            <option key={s} value={s}>{s}</option>
          ))}
        </select>

        <input
          type="text"
          placeholder="Tag..."
          value={tagFilter}
          onChange={(e) => setFilter('tag', e.target.value)}
          style={{ width: 120 }}
        />

        <input
          type="text"
          placeholder="Search control ID or name..."
          value={searchText}
          onChange={(e) => setFilter('q', e.target.value)}
          style={{ flex: 1, minWidth: 180 }}
        />
      </div>

      {isLoading && <p>Loading controls...</p>}
      {error && <p className="error">Failed to load controls: {(error as Error).message}</p>}
      {!isLoading && controls.length === 0 && (
        <p className="muted">No controls match the current filters.</p>
      )}

      {controls.length > 0 && (
        <table className="table">
          <thead>
            <tr>
              <th>Control ID</th>
              <th>Name</th>
              <th>Severity</th>
              <th>Engine</th>
              <th>Frameworks</th>
              <th>Tags</th>
            </tr>
          </thead>
          <tbody>
            {controls.map((c) => {
              const versions = Array.isArray(c.engine_versions) ? c.engine_versions : [];
              const tags = Array.isArray(c.tags) ? c.tags : [];
              return (
                <tr
                  key={c.control_id}
                  style={{ cursor: 'pointer' }}
                  onClick={() => setSelectedControl(c)}
                >
                  <td style={{ fontFamily: 'monospace', fontSize: 13 }}>{c.control_id}</td>
                  <td>{c.name}</td>
                  <td>
                    {c.severity
                      ? <SeverityBadge severity={c.severity} />
                      : <span className="muted">{'\u2014'}</span>}
                  </td>
                  <td>
                    <span title={versions.length > 0 ? `Versions: ${versions.join(', ')}` : ''}>
                      {c.engine}
                    </span>
                  </td>
                  <td>
                    {c.frameworks.map((fw) => (
                      <FrameworkChip
                        key={`${fw.bundle_id}-${fw.section}`}
                        bundleName={fw.bundle_name}
                        section={fw.section}
                      />
                    ))}
                  </td>
                  <td>
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
                      : <span className="muted">{'\u2014'}</span>}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}

      {selectedControl && (
        <ControlDetailDrawer
          control={selectedControl}
          onClose={() => setSelectedControl(null)}
        />
      )}
    </div>
  );
}

function ProfilesPlaceholder() {
  return (
    <section>
      <h2>Custom profiles</h2>
      <p className="muted">
        Custom compliance profiles coming in Level 3. Build tenant-specific
        bundles by selecting controls from the catalog.
      </p>
    </section>
  );
}
