import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  listNotificationChannels,
  createNotificationChannel,
  updateNotificationChannel,
  deleteNotificationChannel,
  type UpsertChannelRequest,
} from '../api/client';
import type { ChannelType, NotificationChannel } from '../api/types';

// CRUD page for D12 notification channels. Webhook + Slack in R1c-a;
// form rejects email/pagerduty for now. Secrets never round-trip — the
// API scrubs them on read and replaces with the sentinel '(set)'. On
// update, secret fields left blank are preserved server-side.

type FormMode = { kind: 'new' } | { kind: 'edit'; channel: NotificationChannel };

export default function NotificationChannels() {
  const queryClient = useQueryClient();
  const { data: channels, isLoading, error } = useQuery({
    queryKey: ['notification-channels'],
    queryFn: listNotificationChannels,
  });

  const [mode, setMode] = useState<FormMode | null>(null);

  const createMut = useMutation({
    mutationFn: createNotificationChannel,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['notification-channels'] });
      setMode(null);
    },
  });

  const updateMut = useMutation({
    mutationFn: ({ id, req }: { id: string; req: UpsertChannelRequest }) => updateNotificationChannel(id, req),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['notification-channels'] });
      setMode(null);
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

  const submitting = createMut.isPending || updateMut.isPending;
  const submitError = createMut.error ?? updateMut.error;

  return (
    <div>
      <div className="page-header">
        <h1>Notification Channels</h1>
        <button
          className="btn btn-primary"
          onClick={() => setMode(mode ? null : { kind: 'new' })}
        >
          {mode ? 'Cancel' : 'New Channel'}
        </button>
      </div>

      {mode && (
        <ChannelForm
          key={mode.kind === 'edit' ? mode.channel.id : 'new'}
          mode={mode}
          submitting={submitting}
          error={submitError ? (submitError as Error).message : null}
          onSubmit={(req) => {
            if (mode.kind === 'edit') updateMut.mutate({ id: mode.channel.id, req });
            else createMut.mutate(req);
          }}
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
                <td style={{ display: 'flex', gap: 4 }}>
                  <button
                    className="btn btn-small"
                    onClick={() => setMode({ kind: 'edit', channel: c })}
                  >
                    Edit
                  </button>
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
  mode: FormMode;
  submitting: boolean;
  error: string | null;
  onSubmit: (req: UpsertChannelRequest) => void;
}

function ChannelForm({ mode, submitting, error, onSubmit }: FormProps) {
  const initial = mode.kind === 'edit' ? mode.channel : null;
  const initialCfg = (initial?.config ?? {}) as Record<string, unknown>;
  const [type, setType] = useState<ChannelType>(initial?.type ?? 'webhook');

  function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    const fd = new FormData(form);
    const name = (fd.get('name') as string).trim();
    const t = fd.get('type') as ChannelType;
    const enabled = fd.get('enabled') !== null;

    let config: Record<string, unknown>;
    if (t === 'webhook') {
      config = {
        url: (fd.get('webhook_url') as string).trim(),
      };
      const secret = (fd.get('webhook_secret') as string).trim();
      if (secret) config.secret = secret;
    } else if (t === 'slack') {
      const slackUrl = (fd.get('slack_url') as string).trim();
      config = {};
      if (slackUrl) config.webhook_url = slackUrl;
    } else {
      return;
    }

    onSubmit({ name, type: t, config, enabled });
  }

  const webhookURL = typeof initialCfg.url === 'string' ? initialCfg.url : '';
  const webhookSecretSet = initialCfg.secret === '(set)';
  const slackURLSet = initialCfg.webhook_url === '(set)';

  return (
    <form className="form-card" onSubmit={handleSubmit}>
      <h3 style={{ marginTop: 0 }}>{initial ? `Edit ${initial.name}` : 'New channel'}</h3>
      <div className="form-group">
        <label htmlFor="name">Name</label>
        <input id="name" name="name" required defaultValue={initial?.name ?? ''} />
      </div>
      <div className="form-group">
        <label htmlFor="type">Type</label>
        <select
          id="type"
          name="type"
          value={type}
          onChange={(e) => setType(e.target.value as ChannelType)}
          disabled={!!initial}
        >
          <option value="webhook">webhook</option>
          <option value="slack">slack</option>
        </select>
        {initial && (
          <p className="muted" style={{ fontSize: 12 }}>
            Type cannot be changed — delete and recreate to switch types.
          </p>
        )}
      </div>
      <div className="form-group">
        <label>
          <input
            type="checkbox"
            name="enabled"
            defaultChecked={initial ? initial.enabled : true}
          />
          {' '}Enabled
        </label>
      </div>
      {type === 'webhook' && (
        <>
          <div className="form-group">
            <label htmlFor="webhook_url">URL</label>
            <input
              id="webhook_url"
              name="webhook_url"
              type="url"
              required
              defaultValue={webhookURL}
              placeholder="https://…"
            />
          </div>
          <div className="form-group">
            <label htmlFor="webhook_secret">Signing secret {initial ? '(leave blank to keep existing)' : '(optional)'}</label>
            <input
              id="webhook_secret"
              name="webhook_secret"
              type="password"
              placeholder={webhookSecretSet ? 'secret is set — leave blank to keep' : 'HMAC secret (stored encrypted)'}
            />
          </div>
        </>
      )}
      {type === 'slack' && (
        <div className="form-group">
          <label htmlFor="slack_url">Slack webhook URL {initial ? '(leave blank to keep existing)' : ''}</label>
          <input
            id="slack_url"
            name="slack_url"
            type="url"
            required={!initial}
            placeholder={slackURLSet ? 'webhook is set — leave blank to keep' : 'https://hooks.slack.com/services/…'}
          />
        </div>
      )}
      <button type="submit" className="btn btn-primary" disabled={submitting}>
        {submitting ? 'Saving…' : initial ? 'Save changes' : 'Create'}
      </button>
      {error && <p className="error">{error}</p>}
    </form>
  );
}
