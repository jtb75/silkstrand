/**
 * Minimal test coverage for the events client glue.
 *
 * React Testing Library / jsdom are not wired into this package yet, so
 * the full `useEventStream` mount-path is exercised manually against
 * stage per ADR 008 PR E's "skip if test infra isn't set up; visual
 * smoke is fine" escape hatch. This file covers the pure-function
 * surface: the stream-token mint request shape.
 */
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { mintStreamToken } from './events';

// vitest's default env is node, which has no localStorage / Response
// globals. Wire up the minimum surface the client helper needs.
const store = new Map<string, string>();
const fakeLocalStorage = {
  getItem: (k: string) => (store.has(k) ? store.get(k)! : null),
  setItem: (k: string, v: string) => { store.set(k, v); },
  removeItem: (k: string) => { store.delete(k); },
  clear: () => { store.clear(); },
} as Storage;

describe('mintStreamToken', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    vi.stubGlobal('localStorage', fakeLocalStorage);
    store.clear();
  });

  it('POSTs to /api/v1/events/stream-tokens with a {filter} wrapper', async () => {
    const fetchSpy = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({ token: 'sig.body.mac', expires_at: '2099-01-01T00:00:00Z' }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      ),
    );
    // Install as the global for the request helper to pick up.
    vi.stubGlobal('fetch', fetchSpy);

    const res = await mintStreamToken({
      kinds: ['agent_log'],
      resource_type: 'agent',
      resource_id: 'a-1',
    });

    expect(res.token).toBe('sig.body.mac');
    expect(fetchSpy).toHaveBeenCalledTimes(1);

    const [url, init] = fetchSpy.mock.calls[0];
    expect(String(url)).toContain('/api/v1/events/stream-tokens');
    expect(init.method).toBe('POST');
    const body = JSON.parse(init.body as string);
    expect(body).toEqual({
      filter: { kinds: ['agent_log'], resource_type: 'agent', resource_id: 'a-1' },
    });
  });

  it('sends an empty filter object when called without arguments', async () => {
    const fetchSpy = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({ token: 't', expires_at: '2099-01-01T00:00:00Z' }),
        { status: 200 },
      ),
    );
    vi.stubGlobal('fetch', fetchSpy);

    await mintStreamToken();
    const init = fetchSpy.mock.calls[0][1];
    expect(JSON.parse(init.body as string)).toEqual({ filter: {} });
  });
});
