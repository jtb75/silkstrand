import { useParams, Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { getTenant, getDataCenter } from '../api/client';
import type { Tenant, DataCenter } from '../api/types';
import StatusBadge from '../components/StatusBadge';

export default function TenantDetail() {
  const { id } = useParams<{ id: string }>();

  const { data: tenant, isLoading, error } = useQuery<Tenant>({
    queryKey: ['tenants', id],
    queryFn: () => getTenant(id!),
    enabled: !!id,
  });

  const { data: dc } = useQuery<DataCenter>({
    queryKey: ['data-centers', tenant?.data_center_id],
    queryFn: () => getDataCenter(tenant!.data_center_id),
    enabled: !!tenant?.data_center_id,
  });

  return (
    <div>
      <Link to="/tenants" className="back-link">Back to Tenants</Link>

      {isLoading && <p>Loading...</p>}
      {error && <p className="error">Failed to load tenant: {(error as Error).message}</p>}

      {tenant && (
        <>
          <div className="page-header">
            <h1>{tenant.name}</h1>
            <StatusBadge status={tenant.status} />
          </div>

          <div className="detail-card">
            <div className="detail-row">
              <span className="detail-label">Status</span>
              <StatusBadge status={tenant.status} />
            </div>
            <div className="detail-row">
              <span className="detail-label">Provisioning</span>
              <StatusBadge status={tenant.provisioning_status} />
            </div>
            <div className="detail-row">
              <span className="detail-label">Data Center</span>
              <span>
                {dc ? (
                  <>
                    <Link to={`/data-centers/${dc.id}`}>{dc.name}</Link>
                    {' '}({dc.region})
                    {' '}
                    <span className={`env-badge env-${dc.environment}`}>{dc.environment}</span>
                  </>
                ) : (
                  <code>{tenant.data_center_id}</code>
                )}
              </span>
            </div>
            <div className="detail-row">
              <span className="detail-label">Tenant ID</span>
              <code title="Tenant ID in the data center's database — use this when debugging on the DC side">
                {tenant.dc_tenant_id || '-'}
              </code>
            </div>
            <div className="detail-row">
              <span className="detail-label">Created</span>
              <span>{new Date(tenant.created_at).toLocaleString()}</span>
            </div>
            <div className="detail-row">
              <span className="detail-label">Updated</span>
              <span>{new Date(tenant.updated_at).toLocaleString()}</span>
            </div>
          </div>

          {tenant.config && Object.keys(tenant.config).length > 0 && (
            <>
              <h2>Configuration</h2>
              <pre className="config-json">{JSON.stringify(tenant.config, null, 2)}</pre>
            </>
          )}
        </>
      )}
    </div>
  );
}
