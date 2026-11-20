package links

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"dfipam/api/internal/validate"
)

func TestHideCommercialClearsMoney(t *testing.T) {
	rental, end := 250000.0, "2027-01-31"
	l := linkDTO{Name: "HO ILL", MonthlyRental: &rental, AnnualCost: &rental, ContractEnd: &end, BillingAccount: "ACC-1", Currency: "LKR"}
	l.hideCommercial()
	if l.MonthlyRental != nil || l.AnnualCost != nil || l.ContractEnd != nil || l.BillingAccount != "" || l.Currency != "" || !l.CommercialHidden {
		t.Fatalf("commercial values left: %+v", l)
	}
	if l.Name != "HO ILL" {
		t.Fatal("other fields must stay")
	}
}

func TestHistoryHidesCommercialChanges(t *testing.T) {
	old := []byte(`{"name":"A","monthly_rental":100,"updated_at":"x"}`)
	now := []byte(`{"name":"B","monthly_rental":200,"updated_at":"y"}`)
	if got := diff(old, now, false); len(got) != 1 || got[0].Field != "name" {
		t.Fatalf("without commercial access: %v", got)
	}
	if got := diff(old, now, true); len(got) != 2 {
		t.Fatalf("with commercial access: %v", got)
	}
}

func TestIPOrNetwork(t *testing.T) {
	cases := map[string]string{"203.0.113.5": "203.0.113.5/32", "203.0.113.0/29": "203.0.113.0/29", "": ""}
	for in, want := range cases {
		var v validate.V
		if got := ipOrNetwork(&v, "f", in, "x"); got != want || !v.OK() {
			t.Errorf("%q: got %q", in, got)
		}
	}
	for _, in := range []string{"203.0.113.5/29", "2001:db8::/32", "nonsense"} {
		var v validate.V
		ipOrNetwork(&v, "f", in, "x")
		if v.OK() {
			t.Errorf("%q was accepted", in)
		}
	}
}

// VPN details go when the type isn't a VPN; the link's segments stay, since
// any link can serve several segments.
func TestVPNFieldsClearedForOtherTypes(t *testing.T) {
	seg := "6f1c1b7e-0a7d-4c55-9a8e-2f4e7f6d3a10"
	in := linkInput{Name: "ILL", SourceSite: "HO", Purpose: "Internet", Role: "primary", Criticality: "high", Status: "active",
		vpn: vpn{RemotePeer: "1.2.3.4", RemoteSubnets: []string{"10.0.0.0/8"}}, LocalSegmentIDs: []string{seg}}
	if err := in.validate(false); err != nil {
		t.Fatal(err)
	}
	if in.RemotePeer != "" || len(in.RemoteSubnets) != 0 {
		t.Fatalf("VPN details kept on a non-VPN link: %+v", in.vpn)
	}
	if len(in.LocalSegmentIDs) != 1 {
		t.Fatalf("segments dropped: %v", in.LocalSegmentIDs)
	}
}

func TestStoreChecksContentAgainstName(t *testing.T) {
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save("contract.pdf", strings.NewReader("<html><script>alert(1)</script>")); !errors.Is(err, errFileType) {
		t.Fatalf("HTML named .pdf: %v", err)
	}
	if _, err := s.Save("page.html", strings.NewReader("<html></html>")); !errors.Is(err, errFileType) {
		t.Fatalf("HTML: %v", err)
	}
	saved, err := s.Save("notes.txt", strings.NewReader("router config without secrets"))
	if err != nil || saved.Size != 29 || saved.ContentType != "text/plain; charset=utf-8" {
		t.Fatalf("text file: %+v %v", saved, err)
	}
	big := bytes.Repeat([]byte("a"), MaxFileBytes+1)
	if _, err := s.Save("big.txt", bytes.NewReader(big)); !errors.Is(err, errFileSize) {
		t.Fatalf("over the limit: %v", err)
	}
}

func TestFileNameIsHarmless(t *testing.T) {
	if got := fileNameOf(`..\..\etc/pass"wd`); got != `pass_wd` {
		t.Fatalf("got %q", got)
	}
}
