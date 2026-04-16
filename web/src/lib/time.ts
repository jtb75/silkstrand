/**
 * Date/time formatting utilities per design-system.md section 8.3.
 *
 * - formatAbsolute: "Apr 12, 2026, 8:45 PM" via Intl.DateTimeFormat.
 * - formatRelative: "5m ago", "2h ago", "3d ago".
 * - Tables with recent data use relative with absolute on hover (title attr).
 * - null / undefined / empty string -> "-".
 */

const absoluteFormatter = new Intl.DateTimeFormat(undefined, {
  year: 'numeric',
  month: 'short',
  day: 'numeric',
  hour: 'numeric',
  minute: '2-digit',
});

/** Format an ISO 8601 timestamp as an absolute locale string. */
export function formatAbsolute(iso: string | null | undefined): string {
  if (!iso) return '-';
  const d = new Date(iso);
  if (isNaN(d.getTime())) return '-';
  return absoluteFormatter.format(d);
}

const MINUTE = 60_000;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;

/** Format an ISO 8601 timestamp as a relative "Xm ago" string. */
export function formatRelative(iso: string | null | undefined): string {
  if (!iso) return '-';
  const d = new Date(iso);
  if (isNaN(d.getTime())) return '-';
  const diff = Date.now() - d.getTime();
  if (diff < 0) return 'just now';
  if (diff < MINUTE) return 'just now';
  if (diff < HOUR) return `${Math.floor(diff / MINUTE)}m ago`;
  if (diff < DAY) return `${Math.floor(diff / HOUR)}h ago`;
  if (diff < 30 * DAY) return `${Math.floor(diff / DAY)}d ago`;
  return formatAbsolute(iso);
}
