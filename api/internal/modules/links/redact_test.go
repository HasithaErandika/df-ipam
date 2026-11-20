package links

import (
	"strings"
	"testing"
)

func TestRedactAuditHidesCommercialValues(t *testing.T) {
	got := string(RedactAudit("network_link", []byte(`{"name":"ISP 1","monthly_rental":125000,"monthlyRental":"125000","contract_ref":"C-9"}`)))
	if strings.Contains(got, "125000") || strings.Contains(got, "C-9") || !strings.Contains(got, "ISP 1") {
		t.Fatalf("redacted: %s", got)
	}
	if got := string(RedactAudit("network_link", []byte(`{"attachment":"contract.pdf","category":"contract"}`))); strings.Contains(got, "contract.pdf") {
		t.Fatalf("contract file name shown: %s", got)
	}
	if got := string(RedactAudit("device", []byte(`{"monthly_rental":1}`))); got != `{"monthly_rental":1}` {
		t.Fatalf("other records must be unchanged: %s", got)
	}
}
