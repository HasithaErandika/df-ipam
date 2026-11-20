package intune

import (
	"encoding/json"
	"testing"
)

func TestEntraIndex(t *testing.T) {
	var devs []EntraDevice
	raw := `[
		{"displayName":"DF-LT-023","accountEnabled":true,"trustType":"AzureAd","approximateLastSignInDateTime":"2026-11-01T08:00:00Z",
		 "registeredOwners":[{"userPrincipalName":"Old.User@dialogfinance.lk","displayName":"Old User"}]},
		{"displayName":"df-lt-023","accountEnabled":true,"trustType":"AzureAd","approximateLastSignInDateTime":"2026-11-18T08:00:00Z",
		 "registeredOwners":[{"userPrincipalName":"Nimal.P@dialogfinance.lk","displayName":"Nimal P"}]},
		{"displayName":"DF-LT-024","accountEnabled":false,"registeredOwners":[{"userPrincipalName":"gone@dialogfinance.lk"}]},
		{"displayName":"DF-PC-HYBRID","accountEnabled":true,"trustType":"ServerAd","registeredOwners":[]},
		{"displayName":"Kamal's MacBook Pro","accountEnabled":true,"trustType":"Workplace","registeredOwners":[{"displayName":"no upn"},{"userPrincipalName":"kamal@dialogfinance.lk"}]}
	]`
	if err := json.Unmarshal([]byte(raw), &devs); err != nil {
		t.Fatal(err)
	}
	idx := entraIndex(devs)
	if len(idx) != 2 {
		t.Fatalf("got %d names, want 2: %v", len(idx), idx)
	}
	if u, _ := idx["DF-LT-023"].owner(); u != "nimal.p@dialogfinance.lk" {
		t.Errorf("DF-LT-023 owner = %q, want the most recently signed-in copy's", u)
	}
	if u, _ := idx["KAMALS-MACBOOK-PRO"].owner(); u != "kamal@dialogfinance.lk" {
		t.Errorf("Mac owner = %q", u)
	}
}
