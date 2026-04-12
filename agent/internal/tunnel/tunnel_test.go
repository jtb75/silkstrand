package tunnel

import (
	"encoding/json"
	"testing"
)

func TestMessageMarshal(t *testing.T) {
	payload, _ := json.Marshal(ScanStartedPayload{ScanID: "scan-123"})
	msg := Message{
		Type:    TypeScanStarted,
		Payload: payload,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Type != TypeScanStarted {
		t.Errorf("Type = %q, want %q", decoded.Type, TypeScanStarted)
	}

	var started ScanStartedPayload
	if err := json.Unmarshal(decoded.Payload, &started); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if started.ScanID != "scan-123" {
		t.Errorf("ScanID = %q, want %q", started.ScanID, "scan-123")
	}
}

func TestDirectivePayloadUnmarshal(t *testing.T) {
	raw := `{
		"scan_id": "s1",
		"bundle_id": "b1",
		"bundle_name": "cis-postgresql-16",
		"bundle_version": "1.0.0",
		"target_id": "t1",
		"target_type": "database",
		"target_identifier": "10.0.1.50:5432",
		"target_config": {"host": "10.0.1.50", "port": 5432}
	}`

	var d DirectivePayload
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if d.ScanID != "s1" {
		t.Errorf("ScanID = %q, want %q", d.ScanID, "s1")
	}
	if d.BundleName != "cis-postgresql-16" {
		t.Errorf("BundleName = %q, want %q", d.BundleName, "cis-postgresql-16")
	}
	if d.TargetType != "database" {
		t.Errorf("TargetType = %q, want %q", d.TargetType, "database")
	}
}

func TestScanResultsPayloadMarshal(t *testing.T) {
	results := json.RawMessage(`{"status": "completed", "summary": {"total": 5, "pass": 5}}`)
	payload := ScanResultsPayload{
		ScanID:  "scan-456",
		Results: results,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded ScanResultsPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.ScanID != "scan-456" {
		t.Errorf("ScanID = %q, want %q", decoded.ScanID, "scan-456")
	}
}

func TestSendNonBlocking(t *testing.T) {
	tun := New("ws://localhost:8080", "test", "key")

	// Fill the send channel
	for i := 0; i < sendChSize; i++ {
		tun.Send(Message{Type: TypeHeartbeat})
	}

	// This should not block even though channel is full
	tun.Send(Message{Type: TypeHeartbeat})
	// If we get here without blocking, test passes
}

func TestBackoffConstants(t *testing.T) {
	if backoffInitial <= 0 {
		t.Error("backoffInitial must be positive")
	}
	if backoffMax < backoffInitial {
		t.Error("backoffMax must be >= backoffInitial")
	}
}
