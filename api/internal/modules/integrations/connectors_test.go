package integrations

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// sourceFiles lists the non-test files matching the patterns, so a file split out
// of a connector or the store stays covered. Each pattern must match something.
func sourceFiles(t *testing.T, patterns ...string) []string {
	t.Helper()
	var out []string
	for _, pattern := range patterns {
		files, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		found := 0
		for _, f := range files {
			if !strings.HasSuffix(f, "_test.go") {
				out = append(out, f)
				found++
			}
		}
		if found == 0 {
			t.Fatalf("no source files match %s", pattern)
		}
	}
	return out
}

// Read-only is enforced three ways: the HTTP client has only GET, the cloud clients
// are reached through interfaces listing read calls, and this test fails if any
// connector source mentions a write operation.
func TestConnectorsContainNoWriteCalls(t *testing.T) {
	forbidden := regexp.MustCompile(`http\.Method(Post|Put|Patch|Delete)|\.(Create|Modify|Delete|Update|Authorize|Revoke|Terminate|Run|Start|Stop|Reboot|Attach|Detach|Associate|Allocate|Release|Replace|BeginCreate|BeginDelete|BeginUpdate|PowerOn|PowerOff|Reconfigure|Destroy)[A-Z]?\w*\(`)
	allowed := map[string]bool{"CreateContainerView(": true, "Destroy(": true, "ReplaceAll(": true} // session-local view, changes nothing in vCenter
	for _, file := range sourceFiles(t, "fortigate*.go", "vcenter*.go", "aws*.go", "azure*.go") {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range forbidden.FindAllString(string(raw), -1) {
			if !allowed[strings.TrimPrefix(m, ".")] {
				t.Errorf("%s: %s", file, m)
			}
		}
	}
}

// Sync output is evidence. If a statement in the only write path mentions one of
// these columns, this fails.
func TestStoreCannotReachOwnershipOrAllocation(t *testing.T) {
	var sql strings.Builder
	for _, file := range sourceFiles(t, "store*.go") {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range regexp.MustCompile("(?s)`[^`]*`").FindAllString(string(raw), -1) {
			sql.WriteString(m)
		}
	}
	for _, column := range []string{"owner_id", "business_service_id", "application_id", "allocation_state", "ip_allocations", "ip_reservations",
		"ip_requests", "site_id", "environment", "sensitivity", "INSERT INTO ip_addresses", "INSERT INTO subnets", "UPDATE subnets"} {
		if strings.Contains(sql.String(), column) {
			t.Errorf("store SQL mentions %s", column)
		}
	}
}

func TestContexts(t *testing.T) {
	r := &Result{Observations: []Observation{{Context: "b"}, {Context: "a"}, {Context: ""}}, Networks: []CloudNetwork{{ExternalID: "b"}},
		Devices: []Device{{VDOMs: []VDOM{{Name: "x", Context: "vdom:x"}}}}}
	if got := strings.Join(r.Contexts(), ","); got != "a,b,vdom:x" {
		t.Fatal(got)
	}
}

func TestCheckSettings(t *testing.T) {
	guid := "11111111-2222-3333-4444-555555555555"
	good := map[string]string{"roleArn": "arn:aws:iam::123456789012:role/df-ipam-read", "externalId": "x", "regions": "ap-south-1, ap-southeast-1"}
	if err := checkSettings(KindAWS, good); err != nil {
		t.Fatal(err)
	}
	for key, bad := range map[string]string{"roleArn": "arn:aws:iam::12:role/x", "regions": "ap south 1"} {
		c := map[string]string{"roleArn": good["roleArn"], "regions": good["regions"]}
		c[key] = bad
		if checkSettings(KindAWS, c) == nil {
			t.Errorf("%s accepted %q", key, bad)
		}
	}
	if checkSettings(KindAWS, map[string]string{"roleArn": good["roleArn"], "regions": " , "}) == nil {
		t.Error("no regions accepted")
	}
	if err := checkSettings(KindAzure, map[string]string{"tenantId": guid, "clientId": guid, "subscriptionId": guid}); err != nil {
		t.Fatal(err)
	}
	if checkSettings(KindAzure, map[string]string{"tenantId": "contoso", "clientId": guid, "subscriptionId": guid}) == nil {
		t.Error("a tenant name was accepted as an ID")
	}
	if err := checkSettings(KindFortiGate, nil); err != nil {
		t.Fatal(err)
	}
}
