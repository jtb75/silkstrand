import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useMutation, useQuery } from '@tanstack/react-query';
import { bulkMapCredentials } from '../api/client';

// Bulk Actions bar — persistent across the Assets / Endpoints / Findings
// tabs whenever the selection is non-empty. Phase-1 actions per ui-shape.md:
//   · Map Credentials — pick a credential_source, POST to
//     /credential-mappings/bulk for every selected endpoint
//   · Create Scan — hand the selected endpoint ids to the new scan
//     definition flow (pre-fill + redirect)
//
// Assets-tab selection is passed as endpoint ids via the onResolveEndpoints
// callback so that host-level selections can expand to all their endpoints.

interface CredentialSourceLite {
  id: string;
  name: string;
  type: string;
}

interface Props {
  selectionCount: number;
  // Caller resolves the current selection (host rows, endpoint rows, or
  // finding rows) down to the endpoint ids needed by the two actions.
  resolveEndpointIds: () => string[];
  onClear: () => void;
  // Optional fetcher for credential sources; defaults to a tiny inline
  // GET so this component stays drop-in. If credentials page supplies a
  // richer list, we can plumb it later.
}

export default function AssetsBulkActions({
  selectionCount,
  resolveEndpointIds,
  onClear,
}: Props) {
  const [modal, setModal] = useState<null | 'credentials'>(null);

  if (selectionCount === 0) return null;

  return (
    <div className="bulk-actions-bar">
      <span>
        <strong>{selectionCount}</strong> selected
      </span>
      <div style={{ display: 'flex', gap: 8 }}>
        <button className="btn btn-sm" onClick={() => setModal('credentials')}>
          Map Credentials
        </button>
        <CreateScanButton resolveEndpointIds={resolveEndpointIds} />
        <button className="btn btn-sm" onClick={onClear}>
          Clear
        </button>
      </div>
      {modal === 'credentials' && (
        <MapCredentialsModal
          endpointIds={resolveEndpointIds()}
          onClose={() => setModal(null)}
        />
      )}
    </div>
  );
}

function CreateScanButton({ resolveEndpointIds }: { resolveEndpointIds: () => string[] }) {
  const navigate = useNavigate();
  return (
    <button
      className="btn btn-sm btn-primary"
      onClick={() => {
        const ids = resolveEndpointIds();
        const qs = new URLSearchParams();
        qs.set('endpoints', ids.join(','));
        navigate(`/scans/definitions/new?${qs.toString()}`);
      }}
    >
      Create Scan
    </button>
  );
}

function MapCredentialsModal({
  endpointIds,
  onClose,
}: {
  endpointIds: string[];
  onClose: () => void;
}) {
  const [sourceId, setSourceId] = useState<string>('');
  const { data: sources, error: listErr, isLoading } = useQuery({
    queryKey: ['credential-sources'],
    queryFn: async () => {
      // Until a dedicated client method lands, hit the endpoint directly.
      // Failures degrade to an empty list with an error banner.
      const res = await fetch('/api/v1/credential-sources', {
        headers: {
          Authorization: `Bearer ${localStorage.getItem('silkstrand_token') ?? ''}`,
        },
      });
      if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
      return (await res.json()) as CredentialSourceLite[];
    },
  });

  const mut = useMutation({
    mutationFn: () =>
      bulkMapCredentials({ endpoint_ids: endpointIds, credential_source_id: sourceId }),
    onSuccess: () => onClose(),
  });

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <header className="modal-header">
          <h3>Map credentials to {endpointIds.length} endpoint{endpointIds.length === 1 ? '' : 's'}</h3>
          <button className="btn btn-sm" onClick={onClose}>×</button>
        </header>
        <div className="modal-body">
          {isLoading && <p>Loading sources…</p>}
          {listErr && <p className="error">{(listErr as Error).message}</p>}
          {sources && (
            <div className="form-group">
              <label htmlFor="cred-source">Credential source</label>
              <select
                id="cred-source"
                value={sourceId}
                onChange={(e) => setSourceId(e.target.value)}
              >
                <option value="">— select —</option>
                {sources.map((s) => (
                  <option key={s.id} value={s.id}>
                    {s.name} ({s.type})
                  </option>
                ))}
              </select>
            </div>
          )}
          {mut.error && <p className="error">{(mut.error as Error).message}</p>}
        </div>
        <footer className="modal-footer">
          <button className="btn" onClick={onClose}>
            Cancel
          </button>
          <button
            className="btn btn-primary"
            disabled={!sourceId || mut.isPending || endpointIds.length === 0}
            onClick={() => mut.mutate()}
          >
            {mut.isPending ? 'Applying…' : `Apply to ${endpointIds.length}`}
          </button>
        </footer>
      </div>
    </div>
  );
}
