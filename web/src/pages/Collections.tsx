import { Fragment, useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  listCollections,
  createCollection,
  updateCollection,
  deleteCollection,
  previewAdhocCollection,
  type UpsertCollectionRequest,
} from '../api/client';
import type {
  Collection,
  CollectionPreview,
  CollectionScope,
  WidgetKind,
} from '../api/types';
import PredicateBuilder, { type Predicate } from '../components/PredicateBuilder';
import { predicateToEnglish } from '../components/predicateToEnglish';

// ADR 006 D5 — Collections replace Asset Sets. Two tabs:
//   · My Collections — every collection the tenant has saved
//   · Dashboard Widgets — subset where is_dashboard_widget=true; per-row
//     widget config (title, widget_kind)
// Inline expand on any row shows a plain-English rendering of the predicate
// (predicateToEnglish) so authors can audit intent without JSON-bashing.

type FormMode = { kind: 'new' } | { kind: 'edit'; c: Collection };
type TabId = 'mine' | 'widgets';

const EXAMPLE_PREDICATE: Predicate = {
  $and: [{ service: 'postgresql' }, { version: { $regex: '^16\\.' } }],
};

export default function Collections() {
  const qc = useQueryClient();
  const [tab, setTab] = useState<TabId>('mine');
  const [mode, setMode] = useState<FormMode | null>(null);
  const [expanded, setExpanded] = useState<string | null>(null);

  const { data: all, isLoading, error } = useQuery({
    queryKey: ['collections'],
    queryFn: () => listCollections(),
  });

  const filtered = useMemo(() => {
    if (!all) return [];
    return tab === 'widgets' ? all.filter((c) => c.is_dashboard_widget) : all;
  }, [all, tab]);

  const createMut = useMutation({
    mutationFn: createCollection,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['collections'] });
      setMode(null);
    },
  });

  const updateMut = useMutation({
    mutationFn: ({ id, req }: { id: string; req: UpsertCollectionRequest }) =>
      updateCollection(id, req),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['collections'] });
      setMode(null);
    },
  });

  const deleteMut = useMutation({
    mutationFn: deleteCollection,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['collections'] }),
  });

  function handleDelete(e: React.MouseEvent, c: Collection) {
    e.stopPropagation();
    if (!window.confirm(`Delete collection "${c.name}"?`)) return;
    deleteMut.mutate(c.id);
  }

  const submitting = createMut.isPending || updateMut.isPending;
  const submitError = createMut.error ?? updateMut.error;

  return (
    <div>
      <div className="page-header">
        <h1>Collections</h1>
        <button
          className="btn btn-primary"
          onClick={() => setMode(mode ? null : { kind: 'new' })}
        >
          {mode ? 'Cancel' : '+ New'}
        </button>
      </div>

      <div className="tab-bar" role="tablist" style={{ marginBottom: 16 }}>
        <button
          role="tab"
          aria-selected={tab === 'mine'}
          className={`btn btn-sm ${tab === 'mine' ? 'btn-primary' : ''}`}
          onClick={() => setTab('mine')}
        >
          My Collections
        </button>
        <button
          role="tab"
          aria-selected={tab === 'widgets'}
          className={`btn btn-sm ${tab === 'widgets' ? 'btn-primary' : ''}`}
          onClick={() => setTab('widgets')}
          style={{ marginLeft: 8 }}
        >
          Dashboard Widgets
        </button>
      </div>

      {mode && (
        <CollectionForm
          key={mode.kind === 'edit' ? mode.c.id : 'new'}
          mode={mode}
          submitting={submitting}
          error={submitError ? (submitError as Error).message : null}
          onSubmit={(req) => {
            if (mode.kind === 'edit') updateMut.mutate({ id: mode.c.id, req });
            else createMut.mutate(req);
          }}
        />
      )}

      {isLoading && <p>Loading…</p>}
      {error && <p className="error">{(error as Error).message}</p>}
      {!isLoading && filtered.length === 0 && (
        <p className="muted">
          {tab === 'widgets'
            ? 'No dashboard widgets yet. Edit a collection and toggle "Show on dashboard".'
            : 'No collections yet. Saved predicates power rules, dashboards, and scans.'}
        </p>
      )}
      {filtered.length > 0 && (
        <table className="table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Type</th>
              {tab === 'widgets' && <th>Widget</th>}
              <th>Count</th>
              <th>Last Updated</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {filtered.map((c) => {
              const open = expanded === c.id;
              return (
                <Fragment key={c.id}>
                  <tr
                    className="clickable-row"
                    onClick={() => setExpanded(open ? null : c.id)}
                  >
                    <td>
                      <strong>{c.name}</strong>
                      {c.description && (
                        <div className="muted" style={{ fontSize: 12 }}>
                          {c.description}
                        </div>
                      )}
                    </td>
                    <td>
                      <span className={`badge badge-scope-${c.scope}`}>{c.scope}</span>
                    </td>
                    {tab === 'widgets' && (
                      <td>
                        {c.widget_title || c.name}
                        <span className="muted" style={{ marginLeft: 6 }}>
                          · {c.widget_kind || 'list'}
                        </span>
                      </td>
                    )}
                    <td>—</td>
                    <td>{new Date(c.updated_at).toLocaleString()}</td>
                    <td style={{ display: 'flex', gap: 4 }}>
                      <button
                        className="btn btn-small"
                        onClick={(e) => {
                          e.stopPropagation();
                          setMode({ kind: 'edit', c });
                        }}
                      >
                        Edit
                      </button>
                      <button
                        className="btn btn-small btn-danger"
                        onClick={(e) => handleDelete(e, c)}
                        disabled={deleteMut.isPending}
                      >
                        Delete
                      </button>
                    </td>
                  </tr>
                  {open && (
                    <tr>
                      <td colSpan={tab === 'widgets' ? 6 : 5} className="muted">
                        <div style={{ padding: '8px 4px' }}>
                          <div style={{ marginBottom: 4, fontSize: 12 }}>Query Preview:</div>
                          <code style={{ whiteSpace: 'pre-wrap' }}>
                            {predicateToEnglish(c.predicate)}
                          </code>
                        </div>
                      </td>
                    </tr>
                  )}
                </Fragment>
              );
            })}
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
  onSubmit: (req: UpsertCollectionRequest) => void;
}

function CollectionForm({ mode, submitting, error, onSubmit }: FormProps) {
  const initial = mode.kind === 'edit' ? mode.c : null;
  const [scope, setScope] = useState<CollectionScope>(initial?.scope ?? 'endpoint');
  const [predicate, setPredicate] = useState<Predicate>(
    initial ? initial.predicate : EXAMPLE_PREDICATE,
  );
  const [widget, setWidget] = useState<boolean>(initial?.is_dashboard_widget ?? false);
  const [widgetKind, setWidgetKind] = useState<WidgetKind>(initial?.widget_kind ?? 'list');
  const [preview, setPreview] = useState<CollectionPreview | null>(null);
  const [previewing, setPreviewing] = useState(false);
  const [previewErr, setPreviewErr] = useState<string | null>(null);

  async function handlePreview() {
    setPreviewErr(null);
    setPreview(null);
    setPreviewing(true);
    try {
      const res = await previewAdhocCollection(scope, predicate);
      setPreview(res);
    } catch (err) {
      setPreviewErr((err as Error).message);
    } finally {
      setPreviewing(false);
    }
  }

  function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const fd = new FormData(e.currentTarget);
    const name = (fd.get('name') as string).trim();
    const description = (fd.get('description') as string).trim() || undefined;
    const widget_title = (fd.get('widget_title') as string)?.trim() || undefined;
    onSubmit({
      name,
      description,
      scope,
      predicate,
      is_dashboard_widget: widget,
      widget_kind: widget ? widgetKind : undefined,
      widget_title: widget ? widget_title : undefined,
    });
  }

  return (
    <form className="form-card" onSubmit={handleSubmit}>
      <h3 style={{ marginTop: 0 }}>
        {initial ? `Edit ${initial.name}` : 'New collection'}
      </h3>
      <div className="form-group">
        <label htmlFor="name">Name</label>
        <input id="name" name="name" required defaultValue={initial?.name ?? ''} />
      </div>
      <div className="form-group">
        <label htmlFor="description">Description</label>
        <input
          id="description"
          name="description"
          defaultValue={initial?.description ?? ''}
          placeholder="optional"
        />
      </div>
      <div className="form-group">
        <label htmlFor="scope">Scope</label>
        <select
          id="scope"
          value={scope}
          onChange={(e) => setScope(e.target.value as CollectionScope)}
        >
          <option value="asset">asset</option>
          <option value="endpoint">endpoint</option>
          <option value="finding">finding</option>
        </select>
      </div>
      <div className="form-group">
        <label>Predicate</label>
        <PredicateBuilder value={predicate} onChange={setPredicate} />
      </div>
      <div className="form-group">
        <label>
          <input
            type="checkbox"
            checked={widget}
            onChange={(e) => setWidget(e.target.checked)}
          />{' '}
          Show on Dashboard
        </label>
      </div>
      {widget && (
        <>
          <div className="form-group">
            <label htmlFor="widget_title">Widget title</label>
            <input
              id="widget_title"
              name="widget_title"
              defaultValue={initial?.widget_title ?? ''}
              placeholder={initial?.name ?? 'optional, defaults to name'}
            />
          </div>
          <div className="form-group">
            <label htmlFor="widget_kind">Widget kind</label>
            <select
              id="widget_kind"
              value={widgetKind}
              onChange={(e) => setWidgetKind(e.target.value as WidgetKind)}
            >
              <option value="list">list</option>
              <option value="count">count</option>
              <option value="chart">chart</option>
            </select>
          </div>
        </>
      )}
      <div style={{ display: 'flex', gap: 8 }}>
        <button
          type="button"
          className="btn"
          disabled={previewing}
          onClick={handlePreview}
        >
          {previewing ? 'Previewing…' : 'Preview matches'}
        </button>
        <button type="submit" className="btn btn-primary" disabled={submitting}>
          {submitting ? 'Saving…' : initial ? 'Save changes' : 'Save'}
        </button>
      </div>
      {preview && (
        <p className="muted" style={{ marginTop: 8 }}>
          Matches {preview.count} {scope}
          {preview.count === 1 ? '' : 's'}.
        </p>
      )}
      {previewErr && <p className="error">{previewErr}</p>}
      {error && <p className="error">{error}</p>}
      <p className="muted" style={{ marginTop: 12, fontSize: 12 }}>
        Preview: <code>{predicateToEnglish(predicate)}</code>
      </p>
    </form>
  );
}
