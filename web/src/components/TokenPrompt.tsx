import { useState } from 'react';
import { setToken } from '../api/client';

/**
 * DEV ONLY: Prompts the user to paste a JWT for local development.
 *
 * Generate a dev token with:
 *   curl -s http://localhost:8080/healthz  # verify API is running
 *
 * Or create one manually with your JWT_SECRET. The payload should be:
 *   {"tenant_id": "00000000-0000-0000-0000-000000000001", "user_id": "dev-user", "exp": <future_timestamp>}
 *
 * Example using jwt-cli (brew install mike-engel/jwt-cli/jwt-cli):
 *   jwt encode --secret "dev-secret-change-in-production" \
 *     '{"tenant_id":"00000000-0000-0000-0000-000000000001","user_id":"dev-user","exp":9999999999}'
 */
export default function TokenPrompt({ onTokenSet }: { onTokenSet: () => void }) {
  const [value, setValue] = useState('');

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    const trimmed = value.trim();
    if (trimmed) {
      setToken(trimmed);
      onTokenSet();
    }
  }

  return (
    <div className="token-prompt">
      <h2>Dev Authentication</h2>
      <p>
        No auth token found. Paste a JWT below to authenticate against the local
        API server.
      </p>
      <p className="token-hint">
        Generate a token with jwt-cli:
        <br />
        <code>
          jwt encode --secret "dev-secret-change-in-production"
          &#123;"tenant_id":"00000000-0000-0000-0000-000000000001","user_id":"dev-user","exp":9999999999&#125;
        </code>
      </p>
      <form onSubmit={handleSubmit}>
        <textarea
          rows={4}
          value={value}
          onChange={(e) => setValue(e.target.value)}
          placeholder="Paste JWT token here..."
          className="token-input"
        />
        <br />
        <button type="submit" className="btn btn-primary">
          Set Token
        </button>
      </form>
    </div>
  );
}
