package scanner

import (
	"context"
	"fmt"
	"net/netip"
	"testing"
	"time"
)

func TestDueFull(t *testing.T) {
	s := New(nil)
	seg := segment{id: "a"}
	if !s.dueFull(seg, "10.0.0.5", false) {
		t.Fatal("a new host must be identified in full")
	}
	if !s.dueFull(seg, "10.0.0.5", false) {
		t.Fatal("a host is due until its identification finishes")
	}
	s.markFull(seg, "10.0.0.5", false)
	if s.dueFull(seg, "10.0.0.5", false) {
		t.Fatal("a host identified moments ago must not be again on a scheduled scan")
	}
	if s.dueFull(seg, "10.0.0.5", true) {
		t.Fatal("a manual scan must not identify a host again within manualFullEvery")
	}
	s.lastFull["a|10.0.0.5"] = fullMark{at: time.Now().Add(-manualFullEvery - time.Minute)}
	if !s.dueFull(seg, "10.0.0.5", true) || s.dueFull(seg, "10.0.0.5", false) {
		t.Fatal("after manualFullEvery only a manual scan identifies the host again")
	}
	seg.checkPorts = true
	s.markFull(seg, "10.0.0.6", true)
	if s.dueFull(seg, "10.0.0.6", false) {
		t.Fatal("ports follow the same schedule once checked")
	}
	s.lastFull["a|10.0.0.7"] = fullMark{at: time.Now()}
	if !s.dueFull(seg, "10.0.0.7", false) {
		t.Fatal("a segment that starts checking ports gets them on its next scan")
	}
	s.lastFull["a|10.0.0.8"] = fullMark{at: time.Now().Add(-fullEvery - time.Minute), portsAt: time.Now()}
	if !s.dueFull(seg, "10.0.0.8", false) {
		t.Fatal("a host is identified in full again after fullEvery")
	}
}

// NetBIOS lists several unique names; the computer's is type 0x00, not the
// signed-in user's (0x03).
func TestComputerName(t *testing.T) {
	entry := func(name string, kind byte, group bool) []byte {
		b := make([]byte, 18)
		copy(b, fmt.Sprintf("%-15s", name))
		b[15] = kind
		if group {
			b[16] = 0x80
		}
		return b
	}
	table := append(append(append(entry("JSMITH", 0x03, false), entry("FINANCE", 0x00, true)...), entry("PC-042", 0x20, false)...), entry("PC-042", 0x00, false)...)
	if got := computerName(table, 4); got != "PC-042" {
		t.Fatalf("got %q, want PC-042", got)
	}
	if got := computerName(entry("JSMITH", 0x03, false), 1); got != "" {
		t.Fatalf("a user name was taken as the computer name: %q", got)
	}
}

// The ARP MAC is the card at this address; NetBIOS may report another card.
// A host that answered only ARP is still recorded.
func TestAddNeighboursPrefersARP(t *testing.T) {
	seg := segment{id: "a", kind: "server"}
	hosts := []netip.Addr{netip.MustParseAddr("10.0.0.5"), netip.MustParseAddr("10.0.0.6")}
	found := []sighting{{ip: "10.0.0.5", mac: "aa:aa:aa:aa:aa:aa", source: "netbios"}}
	arp := map[string]string{"10.0.0.5": "00:11:22:33:44:55", "10.0.0.6": "00:11:22:33:44:66", "10.9.9.9": "00:11:22:33:44:77"}
	out := addNeighbours(context.Background(), seg, hosts, found, arp, nil)
	if len(out) != 2 || out[0].mac != "00:11:22:33:44:55" {
		t.Fatalf("ARP did not win: %+v", out)
	}
	if out[1].ip != "10.0.0.6" || out[1].source != "arp" || !out[1].static {
		t.Fatalf("silent host not recorded from ARP: %+v", out[1])
	}
}
