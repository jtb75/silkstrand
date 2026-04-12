import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { listDataCenters, createDataCenter, deleteDataCenter } from '../api/client';
import type { DataCenter, CreateDataCenterRequest } from '../api/types';
import StatusBadge from '../components/StatusBadge';

export default function DataCenters() {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [showForm, setShowForm] = useState(false);

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

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteDataCenter(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['data-centers'] });
    },
  });

  function handleCreate(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    const formData = new FormData(form);

    createMutation.mutate({
      name: formData.get('name') as string,
      region: formData.get('region') as string,
      api_url: formData.get('api_url') as string,
      api_key: formData.get('api_key') as string,
    });
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

      {isLoading && <p>Loading...</p>}
      {error && <p className="error">Failed to load data centers: {(error as Error).message}</p>}
      {!isLoading && dataCenters && dataCenters.length === 0 && <p>No data centers registered.</p>}
      {dataCenters && dataCenters.length > 0 && (
        <table className="table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Region</th>
              <th>Status</th>
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
                  <StatusBadge status={dc.status} />
                </td>
                <td>{dc.tenant_count}</td>
                <td>{new Date(dc.created_at).toLocaleString()}</td>
                <td>
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
