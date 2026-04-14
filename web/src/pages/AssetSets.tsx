import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  listAssetSets,
  createAssetSet,
  updateAssetSet,
  deleteAssetSet,
  previewAssetSetAdhoc,
  type UpsertAssetSetRequest,
} from '../api/client';
import type { AssetSet, AssetSetPreview } from '../api/types';
import PredicateBuilder, { type Predicate } from '../components/PredicateBuilder';

const EXAMPLE_PREDICATE: Predicate = {
  $and: [
    { service: 'postgresql' },
    { version: { $regex: '^16\\.' } },
  ],
};

type FormMode = { kind: 'new' } | { kind: 'edit'; set: AssetSet };

export default function AssetSets() {
  const queryClient = useQueryClient();
  const { data: sets, isLoading, error } = useQuery({
    queryKey: ['asset-sets'],
    queryFn: listAssetSets,
  });

  const [mode, setMode] = useState<FormMode | null>(null);

  const createMut = useMutation({
    mutationFn: createAssetSet,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['asset-sets'] });
      setMode(null);
    },
  });

  const updateMut = useMutation({
    mutationFn: ({ id, req }: { id: string; req: UpsertAssetSetRequest }) => updateAssetSet(id, req),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['asset-sets'] });
      setMode(null);
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

  const submitting = createMut.isPending || updateMut.isPending;
  const submitError = createMut.error ?? updateMut.error;

  return (
    <div>
      <div className="page-header">
        <h1>Asset Sets</h1>
        <button
          className="btn btn-primary"
          onClick={() => setMode(mode ? null : { kind: 'new' })}
        >
          {mode ? 'Cancel' : 'New Set'}
        </button>
      </div>

      {mode && (
        <AssetSetForm
          key={mode.kind === 'edit' ? mode.set.id : 'new'}
          mode={mode}
          submitting={submitting}
          error={submitError ? (submitError as Error).message : null}
          onSubmit={(req) => {
            if (mode.kind === 'edit') updateMut.mutate({ id: mode.set.id, req });
            else createMut.mutate(req);
          }}
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
                <td style={{ display: 'flex', gap: 4 }}>
                  <button
                    className="btn btn-small"
                    onClick={() => setMode({ kind: 'edit', set: s })}
                  >
                    Edit
                  </button>
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
  mode: FormMode;
  submitting: boolean;
  error: string | null;
  onSubmit: (req: UpsertAssetSetRequest) => void;
}

function AssetSetForm({ mode, submitting, error, onSubmit }: FormProps) {
  const initial = mode.kind === 'edit' ? mode.set : null;
  const [predicate, setPredicate] = useState<Predicate>(initial ? initial.predicate : EXAMPLE_PREDICATE);
  const [parseErr, setParseErr] = useState<string | null>(null);
  const [preview, setPreview] = useState<AssetSetPreview | null>(null);
  const [previewing, setPreviewing] = useState(false);

  async function handlePreview() {
    setParseErr(null);
    setPreview(null);
    setPreviewing(true);
    try {
      const res = await previewAssetSetAdhoc(predicate);
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
    setParseErr(null);
    onSubmit({ name, description: desc, predicate });
  }

  return (
    <form className="form-card" onSubmit={handleSubmit}>
      <h3 style={{ marginTop: 0 }}>{initial ? `Edit ${initial.name}` : 'New asset set'}</h3>
      <div className="form-group">
        <label htmlFor="name">Name</label>
        <input id="name" name="name" required defaultValue={initial?.name ?? ''} placeholder="postgres-candidates" />
      </div>
      <div className="form-group">
        <label htmlFor="description">Description</label>
        <input id="description" name="description" defaultValue={initial?.description ?? ''} placeholder="optional" />
      </div>
      <div className="form-group">
        <label>Predicate</label>
        <PredicateBuilder value={predicate} onChange={setPredicate} />
      </div>
      <div style={{ display: 'flex', gap: 8 }}>
        <button type="button" className="btn" disabled={previewing} onClick={handlePreview}>
          {previewing ? 'Previewing…' : 'Preview matches'}
        </button>
        <button type="submit" className="btn btn-primary" disabled={submitting}>
          {submitting ? 'Saving…' : initial ? 'Save changes' : 'Save'}
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
