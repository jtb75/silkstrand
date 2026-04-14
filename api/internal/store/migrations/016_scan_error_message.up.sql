-- Surface scan failure reasons in the UI: agents already send a reason
-- string via the scan_error WSS message, but it was logged to slog only.
-- Persist it on the scans row so the Scan Results page can render it.
ALTER TABLE scans ADD COLUMN error_message TEXT;
