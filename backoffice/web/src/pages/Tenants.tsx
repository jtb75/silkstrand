import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  listTenants,
  listDataCenters,
  createTenant,
  updateTenantStatus,
  retryTenantProvisioning,
} from '../api/client';
import type { Tenant, DataCenter, CreateTenantRequest } from '../api/types';
import StatusBadge from '../components/StatusBadge';

export default function Tenants() {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [showForm, setShowForm] = useState(false);
  const [filterDc, setFilterDc] = useState('');

  const { data: tenants, isLoading, error } = useQuery<Tenant[]>({
    queryKey: ['tenants', { data_center_id: filterDc || undefined }],
    queryFn: () => listTenants(filterDc || undefined),
  });

  const { data: dataCenters } = useQuery<DataCenter[]>({
    queryKey: ['data-centers'],
    queryFn: listDataCenters,
  });

  const createMutation = useMutation({
    mutationFn: (req: CreateTenantRequest) => createTenant(req),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['tenants'] });
      setShowForm(false);
    },
  });

  const statusMutation = useMutation({
    mutationFn: ({ id, status }: { id: string; status: 'active' | 'suspended' }) =>
      updateTenantStatus(id, { status }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['tenants'] });
    },
  });

  const retryMutation = useMutation({
    mutationFn: (id: string) => retryTenantProvisioning(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['tenants'] });
    },
  });

  function handleCreate(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    const formData = new FormData(form);

    createMutation.mutate({
      data_center_id: formData.get('data_center_id') as string,
      name: formData.get('name') as string,
    });
  }

  function handleToggleStatus(tenant: Tenant) {
    const newStatus = tenant.status === 'active' ? 'suspended' : 'active';
    statusMutation.mutate({ id: tenant.id, status: newStatus });
  }

  return (
    <div>
      <div className="page-header">
        <h1>Tenants</h1>
        <button className="btn btn-primary" onClick={() => setShowForm(!showForm)}>
          {showForm ? 'Cancel' : 'New Tenant'}
        </button>
      </div>

      {showForm && (
        <form className="form-card" onSubmit={handleCreate}>
          <div className="form-group">
            <label htmlFor="data_center_id">Data Center</label>
            <select id="data_center_id" name="data_center_id" required>
              <option value="">Select a data center</option>
              {dataCenters?.map((dc) => (
                <option key={dc.id} value={dc.id}>
                  {dc.name} ({dc.region})
                </option>
              ))}
            </select>
          </div>
          <div className="form-group">
            <label htmlFor="name">Tenant Name</label>
            <input id="name" name="name" type="text" required placeholder="e.g. Acme Corp" />
          </div>
          <button type="submit" className="btn btn-primary" disabled={createMutation.isPending}>
            {createMutation.isPending ? 'Creating...' : 'Create Tenant'}
          </button>
          {createMutation.error && (
            <p className="error">{(createMutation.error as Error).message}</p>
          )}
        </form>
      )}

      <div className="filter-bar">
        <label htmlFor="filter-dc">Filter by Data Center:</label>
        <select
          id="filter-dc"
          value={filterDc}
          onChange={(e) => setFilterDc(e.target.value)}
        >
          <option value="">All</option>
          {dataCenters?.map((dc) => (
            <option key={dc.id} value={dc.id}>
              {dc.name}
            </option>
          ))}
        </select>
      </div>

      {isLoading && <p>Loading...</p>}
      {error && <p className="error">Failed to load tenants: {(error as Error).message}</p>}
      {!isLoading && tenants && tenants.length === 0 && <p>No tenants found.</p>}
      {tenants && tenants.length > 0 && (
        <table className="table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Status</th>
              <th>Provisioning</th>
              <th>DC Tenant ID</th>
              <th>Created</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {tenants.map((t) => (
              <tr
                key={t.id}
                className="clickable-row"
                onClick={() => navigate(`/tenants/${t.id}`)}
              >
                <td>{t.name}</td>
                <td><StatusBadge status={t.status} /></td>
                <td><StatusBadge status={t.provisioning_status} /></td>
                <td className="text-muted">{t.dc_tenant_id || '-'}</td>
                <td>{new Date(t.created_at).toLocaleString()}</td>
                <td>
                  <button
                    className="btn btn-sm"
                    onClick={(e) => {
                      e.stopPropagation();
                      handleToggleStatus(t);
                    }}
                    disabled={statusMutation.isPending}
                  >
                    {t.status === 'active' ? 'Suspend' : 'Activate'}
                  </button>
                  {t.provisioning_status === 'failed' && (
                    <button
                      className="btn btn-primary btn-sm"
                      style={{ marginLeft: 6 }}
                      onClick={(e) => {
                        e.stopPropagation();
                        retryMutation.mutate(t.id);
                      }}
                      disabled={retryMutation.isPending}
                    >
                      Retry
                    </button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
