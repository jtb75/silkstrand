import { useEffect, useState, type FormEvent } from 'react';
import { authApi, type TenantMember } from '../api/authClient';
import { getToken } from '../api/client';
import { useAuth } from '../auth/useAuth';

/**
 * Custom Team page — replaces Clerk's OrganizationProfile. Admins can
 * invite users (email + role), see existing members, and remove them.
 * Members see a read-only list.
 */
export default function Team() {
  const { active, user } = useAuth();
  const isAdmin = active?.role === 'admin';

  const [members, setMembers] = useState<TenantMember[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState<string | null>(null);

  const [inviteEmail, setInviteEmail] = useState('');
  const [inviteRole, setInviteRole] = useState<'admin' | 'member'>('member');
  const [inviteBusy, setInviteBusy] = useState(false);
  const [inviteMsg, setInviteMsg] = useState<string | null>(null);

  async function refresh() {
    const token = getToken();
    if (!token) return;
    setLoading(true);
    try {
      const m = await authApi.listMembers(token);
      setMembers(m);
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => { void refresh(); }, []);

  async function submitInvite(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const token = getToken();
    if (!token) return;
    setInviteBusy(true);
    setInviteMsg(null);
    try {
      await authApi.invite(token, inviteEmail.trim().toLowerCase(), inviteRole);
      setInviteMsg(`Invitation sent to ${inviteEmail}.`);
      setInviteEmail('');
    } catch (e) {
      setInviteMsg((e as Error).message);
    } finally {
      setInviteBusy(false);
    }
  }

  async function remove(userId: string, email: string) {
    if (!confirm(`Remove ${email} from this tenant?`)) return;
    const token = getToken();
    if (!token) return;
    try {
      await authApi.removeMember(token, userId);
      await refresh();
    } catch (e) {
      alert((e as Error).message);
    }
  }

  return (
    <div>
      <h1>Team</h1>
      <p className="muted">Manage users in your tenant.</p>

      {isAdmin && (
        <section style={{ marginTop: 24 }}>
          <h2>Invite user</h2>
          <form onSubmit={submitInvite} style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
            <input
              type="email" required placeholder="user@example.com"
              value={inviteEmail} onChange={(e) => setInviteEmail(e.target.value)}
              style={{ flex: 1 }}
            />
            <select value={inviteRole} onChange={(e) => setInviteRole(e.target.value as 'admin' | 'member')}>
              <option value="member">Member</option>
              <option value="admin">Admin</option>
            </select>
            <button className="btn btn-primary" disabled={inviteBusy}>
              {inviteBusy ? 'Sending…' : 'Send invite'}
            </button>
          </form>
          {inviteMsg && <p style={{ marginTop: 8 }}>{inviteMsg}</p>}
        </section>
      )}

      <section style={{ marginTop: 32 }}>
        <h2>Members</h2>
        {loading && <p>Loading…</p>}
        {err && <p className="error">{err}</p>}
        {!loading && !err && (
          <table className="table">
            <thead>
              <tr>
                <th>Email</th>
                <th>Role</th>
                <th>Joined</th>
                {isAdmin && <th></th>}
              </tr>
            </thead>
            <tbody>
              {members.map((m) => (
                <tr key={m.user_id}>
                  <td>{m.email}{user?.id === m.user_id && <span className="muted"> (you)</span>}</td>
                  <td>{m.role}</td>
                  <td>{new Date(m.created_at).toLocaleDateString()}</td>
                  {isAdmin && (
                    <td>
                      {user?.id !== m.user_id && (
                        <button className="btn btn-sm btn-danger" onClick={() => remove(m.user_id, m.email)}>
                          Remove
                        </button>
                      )}
                    </td>
                  )}
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
    </div>
  );
}
