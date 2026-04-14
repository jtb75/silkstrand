import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  listNotificationChannels,
  createNotificationChannel,
  deleteNotificationChannel,
  type UpsertChannelRequest,
} from '../api/client';
import type { ChannelType, NotificationChannel } from '../api/types';

// Minimal CRUD page for D12 notification channels. Webhook + Slack in
// R1c-a; form rejects email/pagerduty for now. Secrets never round-trip
// — the API scrubs them on read and replaces with the sentinel '(set)'.
export default function NotificationChannels() {
  const queryClient = useQueryClient();
  const { data: channels, isLoading, error } = useQuery({
    queryKey: ['notification-channels'],
    queryFn: listNotificationChannels,
  });

  const [showForm, setShowForm] = useState(false);

  const createMut = useMutation({
    mutationFn: createNotificationChannel,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['notification-channels'] });
      setShowForm(false);
    },
  });

  const deleteMut = useMutation({
    mutationFn: deleteNotificationChannel,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['notification-channels'] }),
  });

  function handleDelete(e: React.MouseEvent, c: NotificationChannel) {
    e.stopPropagation();
    if (!window.confirm(`Delete notification channel ${c.name}?`)) return;
    deleteMut.mutate(c.id);
  }

  return (
    <div>
      <div className="page-header">
        <h1>Notification Channels</h1>
        <button className="btn btn-primary" onClick={() => setShowForm(!showForm)}>
          {showForm ? 'Cancel' : 'New Channel'}
        </button>
      </div>

      {showForm && (
        <ChannelForm
          submitting={createMut.isPending}
          error={createMut.error ? (createMut.error as Error).message : null}
          onSubmit={(req) => createMut.mutate(req)}
        />
      )}

      {isLoading && <p>Loading…</p>}
      {error && <p className="error">{(error as Error).message}</p>}
      {!isLoading && channels && channels.length === 0 && (
        <p className="muted">No notification channels yet. Create one to route rule actions.</p>
      )}
      {channels && channels.length > 0 && (
        <table className="table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Type</th>
              <th>Config</th>
              <th>Enabled</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {channels.map((c) => (
              <tr key={c.id}>
                <td>{c.name}</td>
                <td><span className={`badge badge-type-${c.type}`}>{c.type}</span></td>
                <td>{renderConfigSummary(c)}</td>
                <td>{c.enabled ? 'yes' : 'no'}</td>
                <td>
                  <button
                    className="btn btn-small btn-danger"
                    onClick={(e) => handleDelete(e, c)}
                    disabled={deleteMut.isPending}
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

function renderConfigSummary(c: NotificationChannel): string {
  const cfg = c.config ?? {};
  if (c.type === 'webhook') {
    const url = typeof cfg.url === 'string' ? cfg.url : '-';
    const secret = typeof cfg.secret === 'string' && cfg.secret === '(set)' ? ' + secret' : '';
    return `${url}${secret}`;
  }
  if (c.type === 'slack') {
    return typeof cfg.webhook_url === 'string' && cfg.webhook_url === '(set)'
      ? 'webhook configured'
      : '—';
  }
  return '—';
}

interface FormProps {
  submitting: boolean;
  error: string | null;
  onSubmit: (req: UpsertChannelRequest) => void;
}

function ChannelForm({ submitting, error, onSubmit }: FormProps) {
  const [type, setType] = useState<ChannelType>('webhook');

  function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    const fd = new FormData(form);
    const name = (fd.get('name') as string).trim();
    const t = fd.get('type') as ChannelType;

    let config: Record<string, unknown>;
    if (t === 'webhook') {
      config = {
        url: (fd.get('webhook_url') as string).trim(),
      };
      const secret = (fd.get('webhook_secret') as string).trim();
      if (secret) config.secret = secret;
    } else if (t === 'slack') {
      config = { webhook_url: (fd.get('slack_url') as string).trim() };
    } else {
      return;
    }

    onSubmit({ name, type: t, config, enabled: true });
  }

  return (
    <form className="form-card" onSubmit={handleSubmit}>
      <div className="form-group">
        <label htmlFor="name">Name</label>
        <input id="name" name="name" required />
      </div>
      <div className="form-group">
        <label htmlFor="type">Type</label>
        <select id="type" name="type" value={type} onChange={(e) => setType(e.target.value as ChannelType)}>
          <option value="webhook">webhook</option>
          <option value="slack">slack</option>
        </select>
      </div>
      {type === 'webhook' && (
        <>
          <div className="form-group">
            <label htmlFor="webhook_url">URL</label>
            <input id="webhook_url" name="webhook_url" type="url" required placeholder="https://…" />
          </div>
          <div className="form-group">
            <label htmlFor="webhook_secret">Signing secret (optional)</label>
            <input id="webhook_secret" name="webhook_secret" type="password" placeholder="HMAC secret (stored encrypted)" />
          </div>
        </>
      )}
      {type === 'slack' && (
        <div className="form-group">
          <label htmlFor="slack_url">Slack webhook URL</label>
          <input id="slack_url" name="slack_url" type="url" required placeholder="https://hooks.slack.com/services/…" />
        </div>
      )}
      <button type="submit" className="btn btn-primary" disabled={submitting}>
        {submitting ? 'Creating…' : 'Create'}
      </button>
      {error && <p className="error">{error}</p>}
    </form>
  );
}
