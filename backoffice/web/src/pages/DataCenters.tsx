import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  listDataCenters,
  createDataCenter,
  updateDataCenter,
  deleteDataCenter,
} from '../api/client';
import type {
  DataCenter,
  CreateDataCenterRequest,
  UpdateDataCenterRequest,
  DCEnvironment,
} from '../api/types';
import StatusBadge from '../components/StatusBadge';
import { relativeTime } from '../lib/time';

export default function DataCenters() {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [showForm, setShowForm] = useState(false);
  const [editing, setEditing] = useState<DataCenter | null>(null);

  const { data: dataCenters, isLoading, error } = useQuery<DataCenter[]>({
    queryKey: ['data-centers'],
    queryFn: listDataCenters,
  });

  const createMutation = useMutation({
    mutationFn: (req: CreateDataCenterRequest) => createDataCenter(req),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['data-centers'] });
      setShowForm(false);
    },
  });

  const updateMutation = useMutation({
    mutationFn: ({ id, req }: { id: string; req: UpdateDataCenterRequest }) =>
      updateDataCenter(id, req),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['data-centers'] });
      setEditing(null);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteDataCenter(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['data-centers'] });
    },
  });

  function handleCreate(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const formData = new FormData(e.currentTarget);
    createMutation.mutate({
      name: formData.get('name') as string,
      region: formData.get('region') as string,
      environment: formData.get('environment') as DCEnvironment,
      api_url: formData.get('api_url') as string,
      api_key: formData.get('api_key') as string,
    });
  }

  function handleEdit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!editing) return;
    const formData = new FormData(e.currentTarget);
    const req: UpdateDataCenterRequest = {
      name: formData.get('name') as string,
      region: formData.get('region') as string,
      environment: formData.get('environment') as DCEnvironment,
      api_url: formData.get('api_url') as string,
    };
    // Only send api_key if user entered one (leave blank to keep existing)
    const apiKey = formData.get('api_key') as string;
    if (apiKey && apiKey.trim() !== '') {
      req.api_key = apiKey;
    }
    updateMutation.mutate({ id: editing.id, req });
  }

  function handleDelete(id: string) {
    if (window.confirm('Delete this data center?')) {
      deleteMutation.mutate(id);
    }
  }

  return (
    <div>
      <div className="page-header">
        <h1>Data Centers</h1>
        <button className="btn btn-primary" onClick={() => setShowForm(!showForm)}>
          {showForm ? 'Cancel' : 'Register Data Center'}
        </button>
      </div>

      {showForm && (
        <form className="form-card" onSubmit={handleCreate}>
          <div className="form-group">
            <label htmlFor="name">Name</label>
            <input id="name" name="name" type="text" required placeholder="e.g. US Central" />
          </div>
          <div className="form-group">
            <label htmlFor="region">Region</label>
            <input id="region" name="region" type="text" required placeholder="e.g. us-central1" />
          </div>
          <div className="form-group">
            <label htmlFor="environment">Environment</label>
            <select id="environment" name="environment" required defaultValue="stage">
              <option value="stage">Stage</option>
              <option value="prod">Prod</option>
            </select>
          </div>
          <div className="form-group">
            <label htmlFor="api_url">API URL</label>
            <input
              id="api_url"
              name="api_url"
              type="url"
              required
              placeholder="https://api-stage.silkstrand.io"
            />
          </div>
          <div className="form-group">
            <label htmlFor="api_key">API Key</label>
            <input id="api_key" name="api_key" type="password" required placeholder="API key" />
          </div>
          <button type="submit" className="btn btn-primary" disabled={createMutation.isPending}>
            {createMutation.isPending ? 'Registering...' : 'Register'}
          </button>
          {createMutation.error && (
            <p className="error">{(createMutation.error as Error).message}</p>
          )}
        </form>
      )}

      {editing && (
        <div className="modal-backdrop" onClick={() => setEditing(null)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <div className="modal-header">
              <h2>Edit Data Center</h2>
              <button className="modal-close" onClick={() => setEditing(null)}>
                &times;
              </button>
            </div>
            <form onSubmit={handleEdit}>
              <div className="form-group">
                <label htmlFor="edit-name">Name</label>
                <input
                  id="edit-name"
                  name="name"
                  type="text"
                  required
                  defaultValue={editing.name}
                />
              </div>
              <div className="form-group">
                <label htmlFor="edit-region">Region</label>
                <input
                  id="edit-region"
                  name="region"
                  type="text"
                  required
                  defaultValue={editing.region}
                />
              </div>
              <div className="form-group">
                <label htmlFor="edit-environment">Environment</label>
                <select
                  id="edit-environment"
                  name="environment"
                  required
                  defaultValue={editing.environment}
                >
                  <option value="stage">Stage</option>
                  <option value="prod">Prod</option>
                </select>
              </div>
              <div className="form-group">
                <label htmlFor="edit-api_url">API URL</label>
                <input
                  id="edit-api_url"
                  name="api_url"
                  type="url"
                  required
                  defaultValue={editing.api_url}
                />
              </div>
              <div className="form-group">
                <label htmlFor="edit-api_key">API Key</label>
                <input
                  id="edit-api_key"
                  name="api_key"
                  type="password"
                  placeholder="Leave blank to keep current key"
                />
              </div>
              <div className="modal-actions">
                <button
                  type="button"
                  className="btn btn-secondary"
                  onClick={() => setEditing(null)}
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  className="btn btn-primary"
                  disabled={updateMutation.isPending}
                >
                  {updateMutation.isPending ? 'Saving...' : 'Save'}
                </button>
              </div>
              {updateMutation.error && (
                <p className="error">{(updateMutation.error as Error).message}</p>
              )}
            </form>
          </div>
        </div>
      )}

      {isLoading && <p>Loading...</p>}
      {error && <p className="error">Failed to load data centers: {(error as Error).message}</p>}
      {!isLoading && dataCenters && dataCenters.length === 0 && <p>No data centers registered.</p>}
      {dataCenters && dataCenters.length > 0 && (
        <table className="table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Region</th>
              <th>Environment</th>
              <th>Status</th>
              <th>Health</th>
              <th>Tenants</th>
              <th>Created</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {dataCenters.map((dc) => (
              <tr
                key={dc.id}
                className="clickable-row"
                onClick={() => navigate(`/data-centers/${dc.id}`)}
              >
                <td>{dc.name}</td>
                <td>{dc.region}</td>
                <td>
                  <span className={`env-badge env-${dc.environment}`}>{dc.environment}</span>
                </td>
                <td>
                  <StatusBadge status={dc.status} />
                </td>
                <td>
                  <StatusBadge status={dc.last_health_status || 'unknown'} />
                  <span className="text-muted" style={{ marginLeft: 8, fontSize: 12 }}>
                    {relativeTime(dc.last_health_check)}
                  </span>
                </td>
                <td>{dc.tenant_count}</td>
                <td>{new Date(dc.created_at).toLocaleString()}</td>
                <td className="row-actions">
                  <button
                    className="btn btn-secondary btn-sm"
                    onClick={(e) => {
                      e.stopPropagation();
                      setEditing(dc);
                    }}
                  >
                    Edit
                  </button>
                  <button
                    className="btn btn-danger btn-sm"
                    onClick={(e) => {
                      e.stopPropagation();
                      handleDelete(dc.id);
                    }}
                    disabled={deleteMutation.isPending}
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
