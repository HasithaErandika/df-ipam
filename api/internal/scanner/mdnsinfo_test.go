package scanner

import (
	"testing"

	"golang.org/x/net/dns/dnsmessage"
)

func TestParseModel(t *testing.T) {
	name := dnsmessage.MustNewName("Nimals-MacBook._device-info._tcp.local.")
	msg := dnsmessage.Message{Header: dnsmessage.Header{Response: true}, Answers: []dnsmessage.Resource{{
		Header: dnsmessage.ResourceHeader{Name: name, Type: dnsmessage.TypeTXT, Class: dnsmessage.ClassINET},
		Body:   &dnsmessage.TXTResource{TXT: []string{"osxvers=23", "model=MacBookPro18,1"}},
	}}}
	packet, err := msg.Pack()
	if err != nil {
		t.Fatal(err)
	}
	if got := parseModel(packet); got != "MacBookPro18,1" {
		t.Fatalf("parseModel = %q", got)
	}
	if got := parseModel(nil); got != "" {
		t.Fatalf("empty packet gave %q", got)
	}
}

func TestPortList(t *testing.T) {
	if got := portList(nil, false); got != "" {
		t.Errorf("unchecked portList = %q", got)
	}
	if got := portList(nil, true); got != "{}" {
		t.Errorf("portList(nil) = %q", got)
	}
	if got := portList([]int{22, 80, 443}, true); got != "{22,80,443}" {
		t.Errorf("portList = %q", got)
	}
}

func TestOSFromTTL(t *testing.T) {
	for ttl, want := range map[int]string{0: "", 63: "Linux, Android or Apple (TTL 64)", 127: "Windows (TTL 128)", 254: "Network device or printer (TTL 255)"} {
		if got := osFromTTL(ttl); got != want {
			t.Errorf("osFromTTL(%d) = %q, want %q", ttl, got, want)
		}
	}
}
