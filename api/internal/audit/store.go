package audit

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// StoredEvent is the DB row shape returned by the list query.
type StoredEvent struct {
	ID           string          `json:"id"`
	TenantID     string          `json:"tenant_id"`
	OccurredAt   time.Time       `json:"occurred_at"`
	EventType    string          `json:"event_type"`
	ActorType    string          `json:"actor_type"`
	ActorID      *string         `json:"actor_id,omitempty"`
	ResourceType *string         `json:"resource_type,omitempty"`
	ResourceID   *string         `json:"resource_id,omitempty"`
	Payload      json.RawMessage `json:"payload"`
}

// ListFilter drives the query for GET /api/v1/audit-events.
type ListFilter struct {
	TenantID     string
	EventType    string
	ActorID      string
	ResourceID   string
	ResourceType string
	Since        *time.Time
	Until        *time.Time
	Limit        int
	CursorTime   *time.Time
	CursorID     string
}

// ListResult is returned by ListAuditEvents.
type ListResult struct {
	Items      []StoredEvent `json:"items"`
	NextCursor string        `json:"next_cursor,omitempty"`
}

// ListAuditEvents queries the partitioned audit_events table with
// optional filters and cursor pagination.
func ListAuditEvents(ctx context.Context, db *sql.DB, f ListFilter) (*ListResult, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	var (
		clauses []string
		args    []any
		n       = 1
	)
	arg := func(v any) string {
		args = append(args, v)
		s := fmt.Sprintf("$%d", n)
		n++
		return s
	}
	clauses = append(clauses, "tenant_id = "+arg(f.TenantID))
	if f.EventType != "" {
		clauses = append(clauses, "event_type = "+arg(f.EventType))
	}
	if f.ActorID != "" {
		clauses = append(clauses, "actor_id = "+arg(f.ActorID))
	}
	if f.ResourceID != "" {
		clauses = append(clauses, "resource_id = "+arg(f.ResourceID))
	}
	if f.ResourceType != "" {
		clauses = append(clauses, "resource_type = "+arg(f.ResourceType))
	}
	if f.Since != nil {
		clauses = append(clauses, "occurred_at >= "+arg(*f.Since))
	}
	if f.Until != nil {
		clauses = append(clauses, "occurred_at <= "+arg(*f.Until))
	}
	if f.CursorTime != nil && f.CursorID != "" {
		clauses = append(clauses,
			fmt.Sprintf("(occurred_at, id) < (%s, %s)", arg(*f.CursorTime), arg(f.CursorID)))
	}

	q := `SELECT id, tenant_id, occurred_at, event_type, actor_type, actor_id, resource_type, resource_id, payload
	      FROM audit_events WHERE ` + strings.Join(clauses, " AND ") +
		` ORDER BY occurred_at DESC, id DESC LIMIT ` + arg(f.Limit+1) // fetch one extra to detect next page

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("querying audit_events: %w", err)
	}
	defer rows.Close()

	items := make([]StoredEvent, 0, f.Limit)
	for rows.Next() {
		var e StoredEvent
		if err := rows.Scan(&e.ID, &e.TenantID, &e.OccurredAt, &e.EventType,
			&e.ActorType, &e.ActorID, &e.ResourceType, &e.ResourceID, &e.Payload); err != nil {
			return nil, fmt.Errorf("scanning audit_events row: %w", err)
		}
		items = append(items, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating audit_events rows: %w", err)
	}

	result := &ListResult{Items: items}
	if len(items) > f.Limit {
		result.Items = items[:f.Limit]
		last := result.Items[f.Limit-1]
		result.NextCursor = EncodeCursor(last.OccurredAt, last.ID)
	}
	return result, nil
}

// EncodeCursor encodes a (time, id) pair into an opaque base64 string.
func EncodeCursor(t time.Time, id string) string {
	raw := t.Format(time.RFC3339Nano) + "|" + id
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// DecodeCursor decodes an opaque cursor into (time, id).
func DecodeCursor(cursor string) (*time.Time, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, "", fmt.Errorf("decoding cursor: %w", err)
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return nil, "", fmt.Errorf("invalid cursor format")
	}
	t, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, "", fmt.Errorf("parsing cursor time: %w", err)
	}
	return &t, parts[1], nil
}
