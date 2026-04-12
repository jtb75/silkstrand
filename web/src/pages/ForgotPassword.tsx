import { useState, type FormEvent } from 'react';
import { Link } from 'react-router-dom';
import { authApi } from '../api/authClient';

export default function ForgotPassword() {
  const [email, setEmail] = useState('');
  const [sent, setSent] = useState(false);
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setBusy(true);
    try {
      await authApi.forgotPassword(email);
    } catch { /* always show success to avoid email enumeration */ }
    setSent(true);
    setBusy(false);
  }

  if (sent) {
    return (
      <div className="auth-card">
        <h1>Check your email</h1>
        <p>If an account exists for <strong>{email}</strong>, we&rsquo;ve sent a reset link. It expires in one hour.</p>
        <p><Link to="/login">Back to sign in</Link></p>
      </div>
    );
  }

  return (
    <div className="auth-card">
      <h1>Forgot password</h1>
      <form onSubmit={submit}>
        <label>Email
          <input type="email" required autoFocus value={email} onChange={(e) => setEmail(e.target.value)} />
        </label>
        <button className="btn btn-primary" disabled={busy}>{busy ? 'Sending…' : 'Send reset link'}</button>
      </form>
      <p style={{ marginTop: 16, fontSize: 14 }}><Link to="/login">Back to sign in</Link></p>
    </div>
  );
}
