import { useState, type FormEvent } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { listAgents, createAgent, rotateAgentKey, deleteAgent, getAgentDownloads } from '../api/client';
import type { Agent, AgentDownloads } from '../api/types';

export default function Agents() {
  const qc = useQueryClient();
  const { data: agents, isLoading, error } = useQuery<Agent[]>({
    queryKey: ['agents'],
    queryFn: listAgents,
  });

  const { data: downloads } = useQuery<AgentDownloads>({
    queryKey: ['agent-downloads'],
    queryFn: getAgentDownloads,
  });

  const [name, setName] = useState('');
  const [newKey, setNewKey] = useState<{ agent: Agent; apiKey: string } | null>(null);

  const createMutation = useMutation({
    mutationFn: (n: string) => createAgent(n),
    onSuccess: (res) => {
      setNewKey({ agent: res.agent, apiKey: res.api_key });
      setName('');
      qc.invalidateQueries({ queryKey: ['agents'] });
    },
  });

  const rotateMutation = useMutation({
    mutationFn: (id: string) => rotateAgentKey(id),
    onSuccess: (res, id) => {
      const agent = agents?.find((a) => a.id === id);
      if (agent) setNewKey({ agent, apiKey: res.api_key });
      qc.invalidateQueries({ queryKey: ['agents'] });
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteAgent(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['agents'] }),
  });

  function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!name.trim()) return;
    createMutation.mutate(name.trim());
  }

  return (
    <div>
      <div className="page-header">
        <h1>Agents</h1>
      </div>

      {downloads && (
        <div className="detail-card" style={{ marginBottom: 24 }}>
          <h2 style={{ marginTop: 0 }}>Download agent</h2>
          <p className="muted" style={{ marginTop: 0 }}>
            One-liner (Linux &amp; macOS):
          </p>
          <pre
            style={{
              background: '#111', color: '#eee', padding: 12, borderRadius: 6,
              overflowX: 'auto', userSelect: 'all',
            }}
          >{downloads.install_cmd}</pre>
          <p className="muted">Or grab a binary directly:</p>
          <ul style={{ margin: 0, paddingLeft: 20 }}>
            {Object.entries(downloads.binaries).map(([platform, url]) => (
              <li key={platform}>
                <a href={url}>{platform}</a>
              </li>
            ))}
          </ul>
        </div>
      )}

      <form onSubmit={submit} className="form-card" style={{ marginBottom: 24 }}>
        <h2 style={{ marginTop: 0 }}>Register a new agent</h2>
        <p className="muted" style={{ marginTop: 0 }}>
          Creates an agent record + one-time API key. Install the silkstrand-agent
          binary on your host and start it with <code>SILKSTRAND_AGENT_ID</code>
          + <code>SILKSTRAND_AGENT_KEY</code> from the response.
        </p>
        <div className="form-group">
          <label htmlFor="name">Name</label>
          <input
            id="name" type="text" required placeholder="e.g. prod-db-01"
            value={name} onChange={(e) => setName(e.target.value)}
          />
        </div>
        <button className="btn btn-primary" disabled={createMutation.isPending}>
          {createMutation.isPending ? 'Registering…' : 'Register agent'}
        </button>
        {createMutation.error && (
          <p className="error">{(createMutation.error as Error).message}</p>
        )}
      </form>

      {newKey && (
        <div
          className="detail-card"
          style={{ marginBottom: 24, borderColor: '#0f766e' }}
        >
          <h3 style={{ marginTop: 0 }}>API key for {newKey.agent.name}</h3>
          <p className="muted">
            Copy this now — it will not be shown again. The key is stored
            only as a hash.
          </p>
          <pre
            style={{
              background: '#f3f4f6', padding: 12, borderRadius: 6,
              userSelect: 'all', overflowX: 'auto',
            }}
          >{newKey.apiKey}</pre>
          <p style={{ fontSize: 13 }}>
            Start the agent with:
          </p>
          <pre style={{ background: '#111', color: '#eee', padding: 12, borderRadius: 6, overflowX: 'auto' }}>
{`SILKSTRAND_AGENT_ID=${newKey.agent.id} \\
SILKSTRAND_AGENT_KEY=${newKey.apiKey} \\
SILKSTRAND_API_URL=wss://<your DC API host> \\
silkstrand-agent`}
          </pre>
          <button className="btn btn-sm" onClick={() => setNewKey(null)}>Dismiss</button>
        </div>
      )}

      {isLoading && <p>Loading…</p>}
      {error && <p className="error">{(error as Error).message}</p>}
      {!isLoading && agents && agents.length === 0 && <p>No agents registered.</p>}

      {agents && agents.length > 0 && (
        <table className="table">
          <thead>
            <tr>
              <th>Name</th>
              <th>ID</th>
              <th>Status</th>
              <th>Version</th>
              <th>Last heartbeat</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {agents.map((a) => (
              <tr key={a.id}>
                <td>{a.name}</td>
                <td className="text-muted" style={{ fontSize: 12 }}>{a.id.slice(0, 8)}…</td>
                <td>{a.status}</td>
                <td>{a.version ?? '—'}</td>
                <td>{a.last_heartbeat ? new Date(a.last_heartbeat).toLocaleString() : '—'}</td>
                <td style={{ textAlign: 'right' }}>
                  <button
                    className="btn btn-sm"
                    style={{ marginRight: 6 }}
                    disabled={rotateMutation.isPending}
                    onClick={() => {
                      if (confirm(`Rotate the API key for ${a.name}? The current key keeps working until the agent reconnects with the new one.`)) {
                        rotateMutation.mutate(a.id);
                      }
                    }}
                  >
                    Rotate key
                  </button>
                  <button
                    className="btn btn-sm btn-danger"
                    disabled={deleteMutation.isPending}
                    onClick={() => {
                      if (confirm(`Delete agent ${a.name}? This revokes its key immediately.`)) {
                        deleteMutation.mutate(a.id);
                      }
                    }}
                  >
                    Delete
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
