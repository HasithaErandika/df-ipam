package scanner

import (
	"testing"

	"golang.org/x/net/dns/dnsmessage"
)

func TestParsePTR(t *testing.T) {
	name := dnsmessage.MustNewName("4.83.26.10.in-addr.arpa.")
	answer := dnsmessage.MustNewName("Galaxy-S23.local.")
	packet, err := (&dnsmessage.Message{
		Header:    dnsmessage.Header{Response: true},
		Questions: []dnsmessage.Question{{Name: name, Type: dnsmessage.TypePTR, Class: dnsmessage.ClassINET}},
		Answers: []dnsmessage.Resource{{
			Header: dnsmessage.ResourceHeader{Name: name, Type: dnsmessage.TypePTR, Class: dnsmessage.ClassINET, TTL: 120},
			Body:   &dnsmessage.PTRResource{PTR: answer},
		}},
	}).Pack()
	if err != nil {
		t.Fatal(err)
	}
	if got := parsePTR(packet); got != "Galaxy-S23" {
		t.Fatalf("got %q", got)
	}
	if got := parsePTR([]byte("junk")); got != "" {
		t.Fatalf("junk parsed as %q", got)
	}
}

func TestMacVendor(t *testing.T) {
	if got := macVendor("a0:b3:39:f0:ab:09"); got != "Intel Corporate" {
		t.Fatalf("got %q", got)
	}
	if got := macVendor("5e:1e:0e:e0:32:68"); got != "" {
		t.Fatalf("a randomised MAC has no maker, got %q", got)
	}
}

func TestDeviceKind(t *testing.T) {
	for urn, want := range map[string]string{
		"urn:schemas-upnp-org:device:MediaRenderer:1":         "TV or media player",
		"urn:schemas-upnp-org:device:Printer:1":               "Printer",
		"urn:schemas-upnp-org:device:InternetGatewayDevice:1": "Router",
		"urn:schemas-upnp-org:device:Basic:1":                 "",
	} {
		if got := deviceKind(urn); got != want {
			t.Errorf("%s: got %q", urn, got)
		}
	}
	if d := (upnpInfo{Maker: "Samsung Electronics", Model: "QA55Q60", Kind: "TV or media player"}).Describe(); d != "Samsung Electronics · QA55Q60 · TV or media player" {
		t.Errorf("describe: %q", d)
	}
}
