import { useState, type FormEvent } from 'react';
import { authApi } from '../api/authClient';
import { getToken } from '../api/client';
import { useAuth } from '../auth/useAuth';

export default function Settings() {
  const { user } = useAuth();

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
    <div>
      <h1>Settings</h1>
      <p className="muted">Signed in as <strong>{user?.email}</strong>.</p>

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
    </div>
  );
}
