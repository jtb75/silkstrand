import { useEffect, useState, type FormEvent } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { authApi } from '../api/authClient';
import { useAuth } from '../auth/useAuth';

type Preview = {
  email: string;
  role: 'admin' | 'member';
  tenant_name: string;
  existing_user: boolean;
};

export default function AcceptInvite() {
  const { acceptInvite } = useAuth();
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const token = params.get('token') ?? '';

  const [preview, setPreview] = useState<Preview | null>(null);
  const [previewErr, setPreviewErr] = useState<string | null>(null);
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!token) return;
    authApi.previewInvitation(token)
      .then(setPreview)
      .catch((e) => setPreviewErr((e as Error).message));
  }, [token]);

  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (preview && !preview.existing_user) {
      if (password.length < 8) { setErr('Password must be at least 8 characters.'); return; }
      if (password !== confirm) { setErr('Passwords do not match.'); return; }
    }
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
  if (previewErr) {
    return <div className="auth-card"><h1>Invitation unavailable</h1><p className="error">{previewErr}</p></div>;
  }
  if (!preview) {
    return <div className="auth-card"><p>Loading invitation…</p></div>;
  }

  const existing = preview.existing_user;

  return (
    <div className="auth-card">
      <h1>{existing ? 'Accept invitation' : 'Accept invitation & sign up'}</h1>
      <p className="muted">
        You&rsquo;ve been invited to <strong>{preview.tenant_name}</strong> as
        {' '}<strong>{preview.role}</strong> ({preview.email}).
      </p>
      {existing ? (
        <p className="muted">Enter your existing SilkStrand password to accept.</p>
      ) : (
        <p className="muted">Set a password to finish creating your account.</p>
      )}

      <form onSubmit={submit}>
        <label>{existing ? 'Password' : 'New password'}
          <input
            type="password" required minLength={existing ? 1 : 8} autoFocus
            value={password} onChange={(e) => setPassword(e.target.value)}
          />
        </label>
        {!existing && (
          <label>Confirm password
            <input
              type="password" required
              value={confirm} onChange={(e) => setConfirm(e.target.value)}
            />
          </label>
        )}
        {err && <p className="error">{err}</p>}
        <button className="btn btn-primary" disabled={busy}>
          {busy ? 'Working…' : existing ? 'Accept & sign in' : 'Accept & create account'}
        </button>
      </form>
    </div>
  );
}
