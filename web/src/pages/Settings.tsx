import { useEffect, useState, type FormEvent } from 'react';
import { authApi } from '../api/authClient';
import { getToken } from '../api/client';
import { useAuth } from '../auth/useAuth';
import Team from './Team';
import Credentials from './Credentials';

// P5-b: Settings is now the single Setup surface (per ui-shape.md §Nav
// Band 4). Team is folded in as a tab; the old top-level Team entry is
// gone. Credentials consolidates DB/host auth + Integrations + Vaults.
// Audit log is a placeholder until ADR 005 / O7.

type Tab = 'profile' | 'team' | 'credentials' | 'audit';

export default function Settings() {
  const { user, active } = useAuth();
  const isAdmin = active?.role === 'admin';
  const [tab, setTab] = useState<Tab>('profile');

  return (
    <div>
      <h1>Settings</h1>
      <p className="muted">Signed in as <strong>{user?.email}</strong>.</p>

      <div className="tab-bar" style={{ display: 'flex', gap: 4, borderBottom: '1px solid #e5e7eb', marginTop: 16 }}>
        <TabButton active={tab === 'profile'} onClick={() => setTab('profile')}>Profile</TabButton>
        {isAdmin && <TabButton active={tab === 'team'} onClick={() => setTab('team')}>Team</TabButton>}
        <TabButton active={tab === 'credentials'} onClick={() => setTab('credentials')}>Credentials</TabButton>
        {isAdmin && <TabButton active={tab === 'audit'} onClick={() => setTab('audit')}>Audit</TabButton>}
      </div>

      <div style={{ marginTop: 24 }}>
        {tab === 'profile' && <ProfileTab />}
        {tab === 'team' && isAdmin && <Team />}
        {tab === 'credentials' && <Credentials />}
        {tab === 'audit' && isAdmin && <AuditPlaceholder />}
      </div>
    </div>
  );
}

function TabButton({ active, children, onClick }: { active: boolean; children: React.ReactNode; onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      style={{
        padding: '8px 16px',
        border: 'none',
        borderBottom: active ? '2px solid #0f766e' : '2px solid transparent',
        background: 'none',
        fontWeight: active ? 600 : 400,
        cursor: 'pointer',
      }}
    >
      {children}
    </button>
  );
}

function ProfileTab() {
  const { user, refresh } = useAuth();

  const [name, setName] = useState(user?.display_name ?? '');
  const [nameMsg, setNameMsg] = useState<string | null>(null);
  const [nameBusy, setNameBusy] = useState(false);

  useEffect(() => { setName(user?.display_name ?? ''); }, [user?.display_name]);

  async function submitName(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const token = getToken();
    if (!token) return;
    setNameBusy(true);
    setNameMsg(null);
    try {
      await authApi.updateProfile(token, name.trim());
      await refresh();
      setNameMsg('Profile updated.');
    } catch (e) {
      setNameMsg((e as Error).message);
    } finally {
      setNameBusy(false);
    }
  }

  const [current, setCurrent] = useState('');
  const [next, setNext] = useState('');
  const [confirm, setConfirm] = useState('');
  const [msg, setMsg] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setErr(null);
    setMsg(null);
    if (next.length < 8) { setErr('New password must be at least 8 characters.'); return; }
    if (next !== confirm) { setErr('Passwords do not match.'); return; }
    const token = getToken();
    if (!token) return;
    setBusy(true);
    try {
      await authApi.changePassword(token, current, next);
      setMsg('Password updated.');
      setCurrent(''); setNext(''); setConfirm('');
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <section style={{ maxWidth: 420 }}>
        <h2>Profile</h2>
        <form onSubmit={submitName}>
          <label>Display name
            <input
              type="text" maxLength={120}
              value={name} onChange={(e) => setName(e.target.value)}
              placeholder="Shown in members lists and emails"
            />
          </label>
          {nameMsg && <p style={{ color: nameMsg.includes('updated') ? '#065f46' : '#b91c1c' }}>{nameMsg}</p>}
          <button className="btn btn-primary" disabled={nameBusy}>
            {nameBusy ? 'Saving…' : 'Save'}
          </button>
        </form>
      </section>

      <section style={{ marginTop: 24, maxWidth: 420 }}>
        <h2>Change password</h2>
        <form onSubmit={submit}>
          <label>Current password
            <input type="password" required value={current} onChange={(e) => setCurrent(e.target.value)} />
          </label>
          <label>New password
            <input type="password" required minLength={8} value={next} onChange={(e) => setNext(e.target.value)} />
          </label>
          <label>Confirm new password
            <input type="password" required value={confirm} onChange={(e) => setConfirm(e.target.value)} />
          </label>
          {err && <p className="error">{err}</p>}
          {msg && <p style={{ color: '#065f46' }}>{msg}</p>}
          <button className="btn btn-primary" disabled={busy}>
            {busy ? 'Saving…' : 'Update password'}
          </button>
        </form>
      </section>
    </>
  );
}

function AuditPlaceholder() {
  return (
    <section>
      <h2>Audit log</h2>
      <p className="muted">
        Audit events will be surfaced here once ADR 005 / O7 ships. Read-only view of
        credential reads, tenant-admin actions, and scan lifecycle events.
      </p>
    </section>
  );
}
