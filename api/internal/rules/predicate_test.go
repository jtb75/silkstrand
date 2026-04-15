package rules

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jtb75/silkstrand/api/internal/model"
)

func mkEndpointView() EndpointView {
	ip := "10.0.0.5"
	host := "db01.example.com"
	env := "production"
	pg := "postgresql"
	v := "16.2"
	now := time.Now()
	return EndpointView{
		Asset: &model.Asset{
			PrimaryIP:    &ip,
			Hostname:     &host,
			Environment:  &env,
			Source:       model.AssetSourceDiscovered,
			ResourceType: model.ResourceTypeHost,
			FirstSeen:    now,
			LastSeen:     now,
		},
		Endpoint: &model.AssetEndpoint{
			Port:         5432,
			Protocol:     "tcp",
			Service:      &pg,
			Version:      &v,
			Technologies: json.RawMessage(`["pgaudit","ssl"]`),
			FirstSeen:    now,
			LastSeen:     now,
		},
	}
}

func mkAsset() *model.Asset {
	ip := "10.0.0.5"
	env := "production"
	return &model.Asset{
		PrimaryIP:    &ip,
		Environment:  &env,
		Source:       model.AssetSourceDiscovered,
		ResourceType: model.ResourceTypeHost,
		FirstSeen:    time.Now(),
		LastSeen:     time.Now(),
	}
}

func mustMatchEndpoint(t *testing.T, predJSON string, want bool) {
	t.Helper()
	got, err := Match(json.RawMessage(predJSON), ScopeEndpoint, mkEndpointView())
	if err != nil {
		t.Fatalf("predicate %s err: %v", predJSON, err)
	}
	if got != want {
		t.Errorf("predicate %s = %v, want %v", predJSON, got, want)
	}
}

func mustMatchAsset(t *testing.T, predJSON string, want bool) {
	t.Helper()
	got, err := Match(json.RawMessage(predJSON), ScopeAsset, mkAsset())
	if err != nil {
		t.Fatalf("predicate %s err: %v", predJSON, err)
	}
	if got != want {
		t.Errorf("predicate %s = %v, want %v", predJSON, got, want)
	}
}

func TestBareScalar(t *testing.T) {
	mustMatchEndpoint(t, `{"service":"postgresql"}`, true)
	mustMatchEndpoint(t, `{"service":"mongodb"}`, false)
	mustMatchEndpoint(t, `{"port":5432}`, true)
	mustMatchEndpoint(t, `{"port":3306}`, false)
}

func TestCIDR(t *testing.T) {
	mustMatchEndpoint(t, `{"ip":{"$cidr":"10.0.0.0/8"}}`, true)
	mustMatchEndpoint(t, `{"ip":{"$cidr":"172.16.0.0/12"}}`, false)
	mustMatchAsset(t, `{"ip":{"$cidr":"10.0.0.0/8"}}`, true)
}

func TestRegex(t *testing.T) {
	mustMatchEndpoint(t, `{"version":{"$regex":"^16\\."}}`, true)
	mustMatchEndpoint(t, `{"version":{"$regex":"^15\\."}}`, false)
}

func TestIn(t *testing.T) {
	mustMatchEndpoint(t, `{"service":{"$in":["postgresql","mysql"]}}`, true)
	mustMatchEndpoint(t, `{"service":{"$in":["mongodb","mssql"]}}`, false)
}

func TestAndOrNot(t *testing.T) {
	mustMatchEndpoint(t, `{"$and":[{"service":"postgresql"},{"environment":"production"}]}`, true)
	mustMatchEndpoint(t, `{"$and":[{"service":"postgresql"},{"environment":"staging"}]}`, false)
	mustMatchEndpoint(t, `{"$or":[{"service":"mongodb"},{"service":"postgresql"}]}`, true)
	mustMatchEndpoint(t, `{"$not":{"service":"mongodb"}}`, true)
	mustMatchEndpoint(t, `{"$not":{"service":"postgresql"}}`, false)
}

func TestTechnologies(t *testing.T) {
	mustMatchEndpoint(t, `{"technologies.ssl":{"$exists":true}}`, true)
	mustMatchEndpoint(t, `{"technologies.kerberos":{"$exists":true}}`, false)
}

func TestEmptyPredicateMatchesAll(t *testing.T) {
	mustMatchEndpoint(t, `{}`, true)
}

func mkFinding() *model.Finding {
	sev := "high"
	cve := "CVE-2024-12345"
	srcID := "nuclei-ssh-weak-cipher"
	now := time.Now()
	return &model.Finding{
		ID:              "11111111-1111-1111-1111-111111111111",
		AssetEndpointID: "22222222-2222-2222-2222-222222222222",
		SourceKind:      model.FindingSourceKindNetworkVuln,
		Source:          "nuclei",
		SourceID:        &srcID,
		CVEID:           &cve,
		Severity:        &sev,
		Title:           "Weak SSH cipher",
		Status:          model.FindingStatusOpen,
		FirstSeen:       now,
		LastSeen:        now,
	}
}

func mustMatchFinding(t *testing.T, predJSON string, want bool) {
	t.Helper()
	got, err := Match(json.RawMessage(predJSON), ScopeFinding, mkFinding())
	if err != nil {
		t.Fatalf("predicate %s err: %v", predJSON, err)
	}
	if got != want {
		t.Errorf("predicate %s = %v, want %v", predJSON, got, want)
	}
}

func TestFindingScope(t *testing.T) {
	mustMatchFinding(t, `{"severity":"high"}`, true)
	mustMatchFinding(t, `{"severity":"low"}`, false)
	mustMatchFinding(t, `{"source":"nuclei"}`, true)
	mustMatchFinding(t, `{"source_kind":"network_vuln"}`, true)
	mustMatchFinding(t, `{"status":"open"}`, true)
	mustMatchFinding(t, `{"status":{"$in":["open","suppressed"]}}`, true)
	mustMatchFinding(t, `{"cve_id":{"$regex":"^CVE-2024-"}}`, true)
	mustMatchFinding(t, `{"$and":[{"severity":"high"},{"status":"open"}]}`, true)
}

func TestFindingScopeWrongSubject(t *testing.T) {
	_, err := Match(json.RawMessage(`{"severity":"high"}`), ScopeFinding, nil)
	if err == nil {
		t.Fatal("finding scope should reject nil subject")
	}
	_, err = Match(json.RawMessage(`{"severity":"high"}`), ScopeFinding, "not a finding")
	if err == nil {
		t.Fatal("finding scope should reject non-*model.Finding subject")
	}
}

func TestWrongSubjectType(t *testing.T) {
	_, err := Match(json.RawMessage(`{"ip":"10.0.0.5"}`), ScopeAsset, "not an asset")
	if err == nil {
		t.Fatal("expected error on wrong subject type")
	}
}
