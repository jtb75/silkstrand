import { useState, type FormEvent } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { useAuth } from '../auth/useAuth';

export default function Login() {
  const { login } = useAuth();
  const navigate = useNavigate();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setErr(null);
    setBusy(true);
    try {
      await login(email, password);
      navigate('/');
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="auth-card">
      <h1>Sign in</h1>
      <form onSubmit={submit}>
        <label>Email
          <input type="email" required autoFocus value={email} onChange={(e) => setEmail(e.target.value)} />
        </label>
        <label>Password
          <input type="password" required value={password} onChange={(e) => setPassword(e.target.value)} />
        </label>
        {err && <p className="error">{err}</p>}
        <button className="btn btn-primary" disabled={busy}>{busy ? 'Signing in…' : 'Sign in'}</button>
      </form>
      <p style={{ marginTop: 16, fontSize: 14 }}>
        <Link to="/forgot-password">Forgot password?</Link>
      </p>
    </div>
  );
}
