import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { listTargets, createTarget, deleteTarget, listAgents } from '../api/client';
import type { Target, CreateTargetRequest, Agent } from '../api/types';
import CredentialModal from '../components/CredentialModal';

export default function Targets() {
  const queryClient = useQueryClient();
  const [showForm, setShowForm] = useState(false);
  const [credTarget, setCredTarget] = useState<Target | null>(null);

  const { data: targets, isLoading, error } = useQuery<Target[]>({
    queryKey: ['targets'],
    queryFn: listTargets,
  });

  const { data: agents } = useQuery<Agent[]>({
    queryKey: ['agents'],
    queryFn: listAgents,
  });

  const createMutation = useMutation({
    mutationFn: (req: CreateTargetRequest) => createTarget(req),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['targets'] });
      setShowForm(false);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteTarget(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['targets'] });
    },
  });

  function handleCreate(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    const formData = new FormData(form);

    let config: Record<string, unknown> = {};
    const configStr = formData.get('config') as string;
    if (configStr.trim()) {
      try {
        config = JSON.parse(configStr);
      } catch {
        alert('Invalid JSON in config field');
        return;
      }
    }

    createMutation.mutate({
      type: formData.get('type') as string,
      identifier: formData.get('identifier') as string,
      environment: (formData.get('environment') as string) || undefined,
      agent_id: (formData.get('agent_id') as string) || undefined,
      config,
    });
  }

  function handleDelete(id: string) {
    if (window.confirm('Delete this target?')) {
      deleteMutation.mutate(id);
    }
  }

  return (
    <div>
      <div className="page-header">
        <h1>Targets</h1>
        <button className="btn btn-primary" onClick={() => setShowForm(!showForm)}>
          {showForm ? 'Cancel' : 'New Target'}
        </button>
      </div>

      {showForm && (
        <form className="form-card" onSubmit={handleCreate}>
          <div className="form-group">
            <label htmlFor="type">Type</label>
            <select id="type" name="type" required>
              <option value="database">database</option>
              <option value="host">host</option>
              <option value="cidr">cidr</option>
              <option value="cloud">cloud</option>
            </select>
          </div>
          <div className="form-group">
            <label htmlFor="identifier">Identifier</label>
            <input
              id="identifier"
              name="identifier"
              type="text"
              required
              placeholder="e.g. studio-local-apps-db"
            />
          </div>
          <div className="form-group">
            <label htmlFor="agent_id">Agent</label>
            <select id="agent_id" name="agent_id" defaultValue="">
              <option value="">— none (unassigned) —</option>
              {agents?.map((a) => (
                <option key={a.id} value={a.id}>{a.name} ({a.status})</option>
              ))}
            </select>
          </div>
          <div className="form-group">
            <label htmlFor="environment">Environment</label>
            <input
              id="environment"
              name="environment"
              type="text"
              placeholder="e.g. production, staging"
            />
          </div>
          <div className="form-group">
            <label htmlFor="config">Config (JSON)</label>
            <textarea
              id="config"
              name="config"
              rows={4}
              placeholder='{"host": "localhost", "port": 5432, "database": "postgres"}'
              defaultValue="{}"
            />
          </div>
          <p className="muted" style={{ fontSize: 13, marginTop: -6 }}>
            After creating, click <strong>Credential</strong> to set the username/password
            the agent will use to connect.
          </p>
          <button
            type="submit"
            className="btn btn-primary"
            disabled={createMutation.isPending}
          >
            {createMutation.isPending ? 'Creating...' : 'Create Target'}
          </button>
          {createMutation.error && (
            <p className="error">{(createMutation.error as Error).message}</p>
          )}
        </form>
      )}

      {isLoading && <p>Loading...</p>}
      {error && <p className="error">Failed to load targets: {(error as Error).message}</p>}
      {!isLoading && targets && targets.length === 0 && <p>No targets yet.</p>}
      {targets && targets.length > 0 && (
        <table className="table">
          <thead>
            <tr>
              <th>Type</th>
              <th>Identifier</th>
              <th>Environment</th>
              <th>Agent</th>
              <th>Created</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {targets.map((t) => {
              const agent = agents?.find((a) => a.id === t.agent_id);
              return (
                <tr key={t.id}>
                  <td><span className="badge badge-type">{t.type}</span></td>
                  <td>{t.identifier}</td>
                  <td>{t.environment || '-'}</td>
                  <td>{agent ? agent.name : (t.agent_id ? t.agent_id.slice(0, 8) + '…' : '-')}</td>
                  <td>{new Date(t.created_at).toLocaleString()}</td>
                  <td style={{ textAlign: 'right' }}>
                    <button
                      className="btn btn-sm"
                      style={{ marginRight: 6 }}
                      onClick={() => setCredTarget(t)}
                    >
                      Credential
                    </button>
                    <button
                      className="btn btn-danger btn-sm"
                      onClick={() => handleDelete(t.id)}
                      disabled={deleteMutation.isPending}
                    >
                      Delete
                    </button>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}

      {credTarget && (
        <CredentialModal target={credTarget} onClose={() => setCredTarget(null)} />
      )}
    </div>
  );
}
