import { useEffect, useState, type FormEvent } from 'react';
import { getTargetCredential, putTargetCredential, deleteTargetCredential } from '../api/client';
import type { Target } from '../api/types';

// Database engines all use the same {username, password} credential shape.
function isDatabaseType(t: string): boolean {
  return ['postgresql', 'aurora_postgresql', 'mssql', 'mongodb',
          'mysql', 'aurora_mysql', 'database'].includes(t);
}

/**
 * Modal for setting/updating a credential on a single target. Credentials
 * are encrypted at rest; the ciphertext is never exposed to the UI —
 * we only know "set" or "not set" + type, never the plaintext.
 *
 * MVP: database targets get a structured form (username/password); other
 * target types get a generic JSON editor.
 */
export default function CredentialModal({
  target,
  onClose,
}: {
  target: Target;
  onClose: () => void;
}) {
  const [hasCred, setHasCred] = useState<boolean | null>(null);
  const [credType, setCredType] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  // Database fields
  const [dbUser, setDbUser] = useState('');
  const [dbPass, setDbPass] = useState('');

  // Generic JSON fallback for non-database types
  const [rawJson, setRawJson] = useState('{\n  \n}');

  useEffect(() => {
    getTargetCredential(target.id)
      .then((r) => { setHasCred(r.set); setCredType(r.type ?? null); })
      .catch((e) => setErr((e as Error).message));
  }, [target.id]);

  async function save(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setBusy(true);
    setErr(null);
    try {
      let type: string;
      let data: Record<string, unknown>;
      if (isDatabaseType(target.type)) {
        if (!dbUser || !dbPass) throw new Error('Username and password are required.');
        type = 'database';
        data = { username: dbUser, password: dbPass };
      } else {
        try {
          data = JSON.parse(rawJson);
        } catch {
          throw new Error('Credential data must be valid JSON.');
        }
        type = target.type;
      }
      await putTargetCredential(target.id, type, data);
      onClose();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  async function clearCred() {
    if (!confirm('Delete the credential for this target?')) return;
    setBusy(true);
    try {
      await deleteTargetCredential(target.id);
      onClose();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="modal-backdrop" onClick={() => !busy && onClose()}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <h2>Credential for {target.identifier}</h2>
        <p className="muted" style={{ marginTop: 0 }}>
          {hasCred === null
            ? 'Loading…'
            : hasCred
              ? `A ${credType ?? 'credential'} is set. Submitting replaces it.`
              : 'No credential set.'}
        </p>

        <form onSubmit={save}>
          {isDatabaseType(target.type) ? (
            <>
              <label>Username
                <input
                  type="text" autoComplete="off" required
                  value={dbUser} onChange={(e) => setDbUser(e.target.value)}
                />
              </label>
              <label>Password
                <input
                  type="password" autoComplete="new-password" required
                  value={dbPass} onChange={(e) => setDbPass(e.target.value)}
                />
              </label>
            </>
          ) : (
            <label>Credential JSON
              <textarea
                rows={6}
                value={rawJson}
                onChange={(e) => setRawJson(e.target.value)}
                style={{ fontFamily: 'monospace' }}
              />
            </label>
          )}

          {err && <p className="error">{err}</p>}
          <div className="modal-actions" style={{ marginTop: 16 }}>
            <button type="button" className="btn" onClick={onClose} disabled={busy}>Cancel</button>
            {hasCred && (
              <button type="button" className="btn btn-danger" onClick={clearCred} disabled={busy}>
                Remove credential
              </button>
            )}
            <button type="submit" className="btn btn-primary" disabled={busy}>
              {busy ? 'Saving…' : hasCred ? 'Replace' : 'Save'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
