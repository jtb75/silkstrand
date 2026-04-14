import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  listAssetSets,
  createAssetSet,
  deleteAssetSet,
  previewAssetSetAdhoc,
  type UpsertAssetSetRequest,
} from '../api/client';
import type { AssetSet, AssetSetPreview } from '../api/types';

const EXAMPLE_PREDICATE = `{
  "$and": [
    { "service": "postgresql" },
    { "version": { "$regex": "^16\\\\." } }
  ]
}`;

export default function AssetSets() {
  const queryClient = useQueryClient();
  const { data: sets, isLoading, error } = useQuery({
    queryKey: ['asset-sets'],
    queryFn: listAssetSets,
  });

  const [showForm, setShowForm] = useState(false);

  const createMut = useMutation({
    mutationFn: createAssetSet,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['asset-sets'] });
      setShowForm(false);
    },
  });

  const deleteMut = useMutation({
    mutationFn: deleteAssetSet,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['asset-sets'] }),
  });

  function handleDelete(e: React.MouseEvent, s: AssetSet) {
    e.stopPropagation();
    if (!window.confirm(`Delete asset set "${s.name}"?`)) return;
    deleteMut.mutate(s.id);
  }

  return (
    <div>
      <div className="page-header">
        <h1>Asset Sets</h1>
        <button className="btn btn-primary" onClick={() => setShowForm(!showForm)}>
          {showForm ? 'Cancel' : 'New Set'}
        </button>
      </div>

      {showForm && (
        <AssetSetForm
          submitting={createMut.isPending}
          error={createMut.error ? (createMut.error as Error).message : null}
          onSubmit={(req) => createMut.mutate(req)}
        />
      )}

      {isLoading && <p>Loading…</p>}
      {error && <p className="error">{(error as Error).message}</p>}
      {!isLoading && sets && sets.length === 0 && (
        <p className="muted">
          No asset sets yet. Saved predicates power one-shot scans and
          (later) scheduled bundle runs.
        </p>
      )}
      {sets && sets.length > 0 && (
        <table className="table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Description</th>
              <th>Created</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {sets.map((s) => (
              <tr key={s.id}>
                <td>{s.name}</td>
                <td>{s.description || '-'}</td>
                <td>{new Date(s.created_at).toLocaleString()}</td>
                <td>
                  <button
                    className="btn btn-small btn-danger"
                    onClick={(e) => handleDelete(e, s)}
                    disabled={deleteMut.isPending}
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

interface FormProps {
  submitting: boolean;
  error: string | null;
  onSubmit: (req: UpsertAssetSetRequest) => void;
}

function AssetSetForm({ submitting, error, onSubmit }: FormProps) {
  const [predicateText, setPredicateText] = useState(EXAMPLE_PREDICATE);
  const [parseErr, setParseErr] = useState<string | null>(null);
  const [preview, setPreview] = useState<AssetSetPreview | null>(null);
  const [previewing, setPreviewing] = useState(false);

  async function handlePreview() {
    setParseErr(null);
    setPreview(null);
    let parsed: Record<string, unknown>;
    try {
      parsed = JSON.parse(predicateText);
    } catch (err) {
      setParseErr('predicate is not valid JSON: ' + (err as Error).message);
      return;
    }
    setPreviewing(true);
    try {
      const res = await previewAssetSetAdhoc(parsed);
      setPreview(res);
    } catch (err) {
      setParseErr((err as Error).message);
    } finally {
      setPreviewing(false);
    }
  }

  function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const fd = new FormData(e.currentTarget);
    const name = (fd.get('name') as string).trim();
    const desc = (fd.get('description') as string).trim() || undefined;
    let parsed: Record<string, unknown>;
    try {
      parsed = JSON.parse(predicateText);
    } catch (err) {
      setParseErr('predicate is not valid JSON: ' + (err as Error).message);
      return;
    }
    setParseErr(null);
    onSubmit({ name, description: desc, predicate: parsed });
  }

  return (
    <form className="form-card" onSubmit={handleSubmit}>
      <div className="form-group">
        <label htmlFor="name">Name</label>
        <input id="name" name="name" required placeholder="postgres-candidates" />
      </div>
      <div className="form-group">
        <label htmlFor="description">Description</label>
        <input id="description" name="description" placeholder="optional" />
      </div>
      <div className="form-group">
        <label htmlFor="predicate">Predicate (JSONB)</label>
        <textarea
          id="predicate"
          name="predicate"
          rows={8}
          value={predicateText}
          onChange={(e) => setPredicateText(e.target.value)}
          style={{ fontFamily: 'monospace', width: '100%' }}
        />
      </div>
      <div style={{ display: 'flex', gap: 8 }}>
        <button type="button" className="btn" disabled={previewing} onClick={handlePreview}>
          {previewing ? 'Previewing…' : 'Preview matches'}
        </button>
        <button type="submit" className="btn btn-primary" disabled={submitting}>
          {submitting ? 'Saving…' : 'Save'}
        </button>
      </div>
      {preview && (
        <p className="muted" style={{ marginTop: 8 }}>
          Matches {preview.count} asset{preview.count === 1 ? '' : 's'}
          {preview.sample.length > 0 && `: ${preview.sample.slice(0, 3).map((a) => a.ip).join(', ')}${preview.count > 3 ? '…' : ''}`}
        </p>
      )}
      {parseErr && <p className="error">{parseErr}</p>}
      {error && <p className="error">{error}</p>}
    </form>
  );
}
