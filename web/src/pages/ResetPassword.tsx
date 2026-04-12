import { useState, type FormEvent } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { authApi } from '../api/authClient';

export default function ResetPassword() {
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const token = params.get('token') ?? '';

  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);

  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (password.length < 8) { setErr('Password must be at least 8 characters.'); return; }
    if (password !== confirm) { setErr('Passwords do not match.'); return; }
    setErr(null);
    setBusy(true);
    try {
      await authApi.resetPassword(token, password);
      setDone(true);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  if (!token) {
    return <div className="auth-card"><h1>Invalid reset link</h1><p>Missing token.</p></div>;
  }

  if (done) {
    return (
      <div className="auth-card">
        <h1>Password updated</h1>
        <p>You can now <Link to="/login">sign in</Link> with your new password.</p>
        <button className="btn btn-primary" onClick={() => navigate('/login')}>Go to sign in</button>
      </div>
    );
  }

  return (
    <div className="auth-card">
      <h1>Reset password</h1>
      <form onSubmit={submit}>
        <label>New password
          <input type="password" required minLength={8} autoFocus value={password} onChange={(e) => setPassword(e.target.value)} />
        </label>
        <label>Confirm password
          <input type="password" required value={confirm} onChange={(e) => setConfirm(e.target.value)} />
        </label>
        {err && <p className="error">{err}</p>}
        <button className="btn btn-primary" disabled={busy}>{busy ? 'Saving…' : 'Update password'}</button>
      </form>
    </div>
  );
}
