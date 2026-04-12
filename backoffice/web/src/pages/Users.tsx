import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  listUsers, getUser, updateUserStatus, deleteUser,
  updateUserMembershipStatus, removeUserMembership,
} from '../api/client';
import type { User, UserDetail } from '../api/types';
import StatusBadge from '../components/StatusBadge';

export default function Users() {
  const qc = useQueryClient();
  const { data: users, isLoading, error } = useQuery<User[]>({
    queryKey: ['users'],
    queryFn: listUsers,
  });
  const [expanded, setExpanded] = useState<string | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<User | null>(null);
  const [deleteText, setDeleteText] = useState('');

  const statusMutation = useMutation({
    mutationFn: ({ id, status }: { id: string; status: 'active' | 'suspended' }) =>
      updateUserStatus(id, status),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['users'] }),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteUser(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['users'] });
      setDeleteTarget(null);
      setDeleteText('');
    },
  });

  return (
    <div>
      <div className="page-header">
        <h1>Users</h1>
      </div>

      {isLoading && <p>Loading...</p>}
      {error && <p className="error">Failed to load users: {(error as Error).message}</p>}
      {!isLoading && users && users.length === 0 && <p>No users yet.</p>}

      {users && users.length > 0 && (
        <table className="table">
          <thead>
            <tr>
              <th></th>
              <th>Email</th>
              <th>Status</th>
              <th>Tenants</th>
              <th>Last login</th>
              <th>Created</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {users.map((u) => (
              <UserRow
                key={u.id}
                user={u}
                expanded={expanded === u.id}
                onToggle={() => setExpanded(expanded === u.id ? null : u.id)}
                onToggleStatus={() =>
                  statusMutation.mutate({
                    id: u.id,
                    status: u.status === 'active' ? 'suspended' : 'active',
                  })
                }
                onDelete={() => { setDeleteTarget(u); setDeleteText(''); }}
              />
            ))}
          </tbody>
        </table>
      )}

      {deleteTarget && (
        <div
          className="modal-backdrop"
          onClick={() => { if (!deleteMutation.isPending) { setDeleteTarget(null); setDeleteText(''); } }}
        >
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <h2>Delete user</h2>
            <p>
              This will permanently delete <strong>{deleteTarget.email}</strong> and
              remove them from all tenants. Pending invitations to other tenants
              will also be invalidated. This cannot be undone.
            </p>
            <p>Type <code>{deleteTarget.email}</code> to confirm:</p>
            <input
              autoFocus type="text" value={deleteText}
              onChange={(e) => setDeleteText(e.target.value)}
              placeholder={deleteTarget.email}
            />
            {deleteMutation.error && (
              <p className="error">{(deleteMutation.error as Error).message}</p>
            )}
            <div className="modal-actions">
              <button className="btn" onClick={() => { setDeleteTarget(null); setDeleteText(''); }}
                disabled={deleteMutation.isPending}>Cancel</button>
              <button
                className="btn btn-danger"
                disabled={deleteText !== deleteTarget.email || deleteMutation.isPending}
                onClick={() => deleteMutation.mutate(deleteTarget.id)}
              >
                {deleteMutation.isPending ? 'Deleting...' : 'Delete user'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function UserRow({
  user, expanded, onToggle, onToggleStatus, onDelete,
}: {
  user: User;
  expanded: boolean;
  onToggle: () => void;
  onToggleStatus: () => void;
  onDelete: () => void;
}) {
  return (
    <>
      <tr>
        <td>
          <button className="btn btn-sm" onClick={onToggle} aria-label="Expand">
            {expanded ? '▾' : '▸'}
          </button>
        </td>
        <td>{user.email}</td>
        <td><StatusBadge status={user.status} /></td>
        <td>{user.tenant_count}</td>
        <td className="text-muted">
          {user.last_login_at ? new Date(user.last_login_at).toLocaleString() : '—'}
        </td>
        <td>{new Date(user.created_at).toLocaleDateString()}</td>
        <td style={{ textAlign: 'right' }}>
          <button className="btn btn-sm" style={{ marginRight: 6 }} onClick={onToggleStatus}>
            {user.status === 'active' ? 'Suspend' : 'Reactivate'}
          </button>
          <button className="btn btn-sm btn-danger" onClick={onDelete}>Delete</button>
        </td>
      </tr>
      {expanded && (
        <tr>
          <td></td>
          <td colSpan={6}>
            <UserDetailPanel userId={user.id} />
          </td>
        </tr>
      )}
    </>
  );
}

function UserDetailPanel({ userId }: { userId: string }) {
  const qc = useQueryClient();
  const { data, isLoading, error } = useQuery<UserDetail>({
    queryKey: ['users', userId],
    queryFn: () => getUser(userId),
  });

  const membershipStatus = useMutation({
    mutationFn: ({ tenantId, status }: { tenantId: string; status: 'active' | 'suspended' }) =>
      updateUserMembershipStatus(userId, tenantId, status),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['users', userId] });
      qc.invalidateQueries({ queryKey: ['users'] });
    },
  });

  const removeMembership = useMutation({
    mutationFn: (tenantId: string) => removeUserMembership(userId, tenantId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['users', userId] });
      qc.invalidateQueries({ queryKey: ['users'] });
    },
  });

  if (isLoading) return <p className="muted">Loading details…</p>;
  if (error) return <p className="error">{(error as Error).message}</p>;
  if (!data) return null;

  return (
    <div style={{ padding: '12px 0' }}>
      <h3 style={{ margin: '0 0 8px' }}>Tenant memberships</h3>
      {data.memberships.length === 0 ? (
        <p className="text-muted">No memberships.</p>
      ) : (
        <table className="table" style={{ marginBottom: 16 }}>
          <thead>
            <tr>
              <th>Tenant</th>
              <th>DC</th>
              <th>Env</th>
              <th>Role</th>
              <th>Status</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {data.memberships.map((m) => (
              <tr key={m.tenant_id}>
                <td>{m.tenant_name}</td>
                <td>{m.dc_name}</td>
                <td><span className={`env-badge env-${m.environment}`}>{m.environment}</span></td>
                <td>{m.role}</td>
                <td><StatusBadge status={m.status} /></td>
                <td style={{ textAlign: 'right' }}>
                  <button
                    className="btn btn-sm"
                    style={{ marginRight: 6 }}
                    onClick={() => membershipStatus.mutate({
                      tenantId: m.tenant_id,
                      status: m.status === 'active' ? 'suspended' : 'active',
                    })}
                    disabled={membershipStatus.isPending}
                  >
                    {m.status === 'active' ? 'Suspend' : 'Reactivate'}
                  </button>
                  <button
                    className="btn btn-sm btn-danger"
                    onClick={() => {
                      if (confirm(`Remove ${data.email} from ${m.tenant_name}?`)) {
                        removeMembership.mutate(m.tenant_id);
                      }
                    }}
                    disabled={removeMembership.isPending}
                  >
                    Remove
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {data.pending_invites.length > 0 && (
        <>
          <h3 style={{ margin: '0 0 8px' }}>Pending invitations</h3>
          <ul style={{ margin: 0, paddingLeft: 20 }}>
            {data.pending_invites.map((i) => (
              <li key={i.id}>
                {i.role} invite, expires {new Date(i.expires_at).toLocaleDateString()}
              </li>
            ))}
          </ul>
        </>
      )}
    </div>
  );
}
