package intune

import (
	"testing"
	"time"
)

func TestNormalMAC(t *testing.T) {
	cases := map[string]string{
		"001A2B3C4D5E":      "00:1a:2b:3c:4d:5e",
		"00-1A-2B-3C-4D-5E": "00:1a:2b:3c:4d:5e",
		"000000000000":      "",
		"":                  "",
		"12345":             "",
	}
	for in, want := range cases {
		if got := normalMAC(in); got != want {
			t.Errorf("normalMAC(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIndex(t *testing.T) {
	old, recent := time.Unix(100, 0), time.Unix(200, 0)
	byMAC, byName := index([]Device{
		{Name: "PC-01", User: "Nimal@dialogfinance.lk", WiFiMAC: "AABBCCDDEEFF", EtherMAC: "001122334455", LastSync: old},
		{Name: "PC-02", User: "kamal@dialogfinance.lk", WiFiMAC: "AABBCCDDEEFF", LastSync: recent},
		{Name: "PC-02", User: "sunil@dialogfinance.lk", LastSync: old},
		{Name: "KIOSK", User: "", EtherMAC: "665544332211"},
	})
	if got := byMAC["aa:bb:cc:dd:ee:ff"].User; got != "kamal@dialogfinance.lk" {
		t.Errorf("a shared MAC goes to the device synced last, got %q", got)
	}
	if got := byMAC["00:11:22:33:44:55"].User; got != "nimal@dialogfinance.lk" {
		t.Errorf("emails are lower-cased, got %q", got)
	}
	if _, ok := byName["PC-02"]; ok {
		t.Error("a repeated computer name must not match")
	}
	if d, ok := byMAC["66:55:44:33:22:11"]; !ok || d.User != "" {
		t.Error("a shared device with no primary user is kept for its details")
	}
	if byName["PC-01"].Name != "PC-01" {
		t.Error("a unique computer name matches")
	}
}

func TestNameKey(t *testing.T) {
	cases := map[string]string{
		"Nimal\u2019s MacBook Pro": "NIMALS-MACBOOK-PRO",
		"Nimal's MacBook Pro":      "NIMALS-MACBOOK-PRO",
		"DESKTOP-7Q2":              "DESKTOP-7Q2",
		" ubuntu-laptop ":          "UBUNTU-LAPTOP",
		"":                         "",
	}
	for in, want := range cases {
		if got := nameKey(in); got != want {
			t.Errorf("nameKey(%q) = %q, want %q", in, got, want)
		}
	}
}
