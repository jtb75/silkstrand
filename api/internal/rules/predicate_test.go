package rules

import (
	"encoding/json"
	"testing"

	"github.com/jtb75/silkstrand/api/internal/model"
)

func mkAsset() *model.DiscoveredAsset {
	pg := "postgresql"
	v := "16.2"
	env := "production"
	return &model.DiscoveredAsset{
		IP:           "10.0.0.5",
		Port:         5432,
		Service:      &pg,
		Version:      &v,
		Environment:  &env,
		Source:       model.AssetSourceDiscovered,
		Technologies: json.RawMessage(`["pgaudit","ssl"]`),
		CVEs:         json.RawMessage(`[{"id":"CVE-2024-4317","severity":"high"}]`),
	}
}

func mustMatch(t *testing.T, predJSON string, want bool) {
	t.Helper()
	got, err := Match(json.RawMessage(predJSON), mkAsset())
	if err != nil {
		t.Fatalf("predicate %s err: %v", predJSON, err)
	}
	if got != want {
		t.Errorf("predicate %s = %v, want %v", predJSON, got, want)
	}
}

func TestBareScalar(t *testing.T) {
	mustMatch(t, `{"service":"postgresql"}`, true)
	mustMatch(t, `{"service":"mongodb"}`, false)
	mustMatch(t, `{"port":5432}`, true)
	mustMatch(t, `{"port":3306}`, false)
}

func TestCIDR(t *testing.T) {
	mustMatch(t, `{"ip":{"$cidr":"10.0.0.0/8"}}`, true)
	mustMatch(t, `{"ip":{"$cidr":"172.16.0.0/12"}}`, false)
}

func TestRegex(t *testing.T) {
	mustMatch(t, `{"version":{"$regex":"^16\\."}}`, true)
	mustMatch(t, `{"version":{"$regex":"^15\\."}}`, false)
}

func TestIn(t *testing.T) {
	mustMatch(t, `{"service":{"$in":["postgresql","mysql"]}}`, true)
	mustMatch(t, `{"service":{"$in":["mongodb","mssql"]}}`, false)
}

func TestAndOrNot(t *testing.T) {
	mustMatch(t, `{"$and":[{"service":"postgresql"},{"environment":"production"}]}`, true)
	mustMatch(t, `{"$and":[{"service":"postgresql"},{"environment":"staging"}]}`, false)
	mustMatch(t, `{"$or":[{"service":"mongodb"},{"service":"postgresql"}]}`, true)
	mustMatch(t, `{"$not":{"service":"mongodb"}}`, true)
	mustMatch(t, `{"$not":{"service":"postgresql"}}`, false)
}

func TestTechnologies(t *testing.T) {
	mustMatch(t, `{"technologies.ssl":{"$exists":true}}`, true)
	mustMatch(t, `{"technologies.kerberos":{"$exists":true}}`, false)
}

func TestCVEs(t *testing.T) {
	mustMatch(t, `{"cves.severity":{"$in":["critical","high"]}}`, true)
	mustMatch(t, `{"cves.severity":{"$in":["critical"]}}`, false)
	mustMatch(t, `{"cves":{"$exists":true}}`, true)
}

func TestShadowITExample(t *testing.T) {
	// From the ADR D2 example, adapted: RDP on a wrong segment.
	rdp := "rdp"
	a := mkAsset()
	a.IP = "10.50.99.7"
	a.Service = &rdp
	got, err := Match(json.RawMessage(
		`{"$and":[
			{"service":"rdp"},
			{"ip":{"$cidr":"10.50.0.0/16"}},
			{"$not":{"ip":{"$cidr":"10.50.10.0/24"}}}
		]}`), a)
	if err != nil || !got {
		t.Errorf("shadow IT predicate didn't match: got=%v err=%v", got, err)
	}
}
