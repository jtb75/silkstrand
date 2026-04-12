import { useState, type FormEvent } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useAuth } from '../auth/useAuth';

export default function AcceptInvite() {
  const { acceptInvite } = useAuth();
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const token = params.get('token') ?? '';

  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (password.length < 8) { setErr('Password must be at least 8 characters.'); return; }
    if (password !== confirm) { setErr('Passwords do not match.'); return; }
    setErr(null);
    setBusy(true);
    try {
      await acceptInvite(token, password);
      navigate('/');
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  if (!token) {
    return <div className="auth-card"><h1>Invalid invitation link</h1><p>Missing token.</p></div>;
  }

  return (
    <div className="auth-card">
      <h1>Accept invitation</h1>
      <p className="muted">Set a password to finish creating your account.</p>
      <form onSubmit={submit}>
        <label>New password
          <input type="password" required minLength={8} value={password} onChange={(e) => setPassword(e.target.value)} />
        </label>
        <label>Confirm password
          <input type="password" required value={confirm} onChange={(e) => setConfirm(e.target.value)} />
        </label>
        {err && <p className="error">{err}</p>}
        <button className="btn btn-primary" disabled={busy}>{busy ? 'Creating…' : 'Accept and sign in'}</button>
      </form>
    </div>
  );
}
