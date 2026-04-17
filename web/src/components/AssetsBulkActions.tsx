import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useMutation, useQuery } from '@tanstack/react-query';
import { bulkMapCredentials, bulkCreateAssetMappings } from '../api/client';
import type { MappingScopeKind } from '../api/client';

// Bulk Actions bar -- persistent across the Assets / Endpoints / Findings
// tabs whenever the selection is non-empty. Actions:
//   - Map Credentials -- scope-aware: endpoint_ids on Endpoints tab,
//     asset_ids on Assets tab
//   - Create Scan -- hand endpoint ids to scan definition flow

interface CredentialSourceLite {
  id: string;
  name: string;
  type: string;
}

interface Props {
  selectionCount: number;
  // Caller resolves the current selection down to IDs for the action.
  resolveEndpointIds: () => string[];
  onClear: () => void;
  // Which scope the current selection represents. Defaults to 'asset_endpoint'
  // for backwards compat (Endpoints tab).
  scopeKind?: MappingScopeKind;
}

export default function AssetsBulkActions({
  selectionCount,
  resolveEndpointIds,
  onClear,
  scopeKind = 'asset_endpoint',
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
        {scopeKind === 'asset_endpoint' && (
          <CreateScanButton resolveEndpointIds={resolveEndpointIds} />
        )}
        <button className="btn btn-sm" onClick={onClear}>
          Clear
        </button>
      </div>
      {modal === 'credentials' && (
        <MapCredentialsModal
          ids={resolveEndpointIds()}
          scopeKind={scopeKind}
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
  ids,
  scopeKind,
  onClose,
}: {
  ids: string[];
  scopeKind: MappingScopeKind;
  onClose: () => void;
}) {
  const [sourceId, setSourceId] = useState<string>('');
  const { data: sources, error: listErr, isLoading } = useQuery({
    queryKey: ['credential-sources'],
    queryFn: async () => {
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
    mutationFn: () => {
      if (scopeKind === 'asset') {
        return bulkCreateAssetMappings(sourceId, ids);
      }
      // Default: endpoint-level
      return bulkMapCredentials({ endpoint_ids: ids, credential_source_id: sourceId });
    },
    onSuccess: () => onClose(),
  });

  const scopeLabel = scopeKind === 'asset' ? 'asset' : 'endpoint';

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <header className="modal-header">
          <h3>Map credentials to {ids.length} {scopeLabel}{ids.length === 1 ? '' : 's'}</h3>
          <button className="btn btn-sm" onClick={onClose}>x</button>
        </header>
        <div className="modal-body">
          {isLoading && <p>Loading sources...</p>}
          {listErr && <p className="error">{(listErr as Error).message}</p>}
          {sources && (
            <div className="form-group">
              <label htmlFor="cred-source">Credential source</label>
              <select
                id="cred-source"
                value={sourceId}
                onChange={(e) => setSourceId(e.target.value)}
              >
                <option value="">-- select --</option>
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
            disabled={!sourceId || mut.isPending || ids.length === 0}
            onClick={() => mut.mutate()}
          >
            {mut.isPending ? 'Applying...' : `Apply to ${ids.length}`}
          </button>
        </footer>
      </div>
    </div>
  );
}
