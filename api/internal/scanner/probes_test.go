package scanner

import (
	"bytes"
	"context"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

// The query must match what Windows' nbtstat -A sends, or Windows stays silent:
// no broadcast flag, and the wildcard "*" padded with zero bytes.
func TestNetBIOSQueryMatchesNbtstat(t *testing.T) {
	q := netbiosQuery()
	if flags := binary.BigEndian.Uint16(q[2:4]); flags != 0 {
		t.Errorf("flags = %#04x, want 0 (unicast, no broadcast bit)", flags)
	}
	if n := binary.BigEndian.Uint16(q[4:6]); n != 1 {
		t.Errorf("questions = %d, want 1", n)
	}
	want := append([]byte{32}, []byte("CKAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")...)
	want = append(want, 0, 0, 33, 0, 1) // end of name, NBSTAT, IN
	if got := q[12:]; !bytes.Equal(got, want) {
		t.Errorf("question = %q, want %q", got, want)
	}
}

// A unicast SSDP search names the device in HOST and carries no MX.
func TestSSDPQueryIsUnicast(t *testing.T) {
	q := string(ssdpQuery(netip.MustParseAddr("192.168.12.8")))
	if !strings.Contains(q, "\r\nHOST: 192.168.12.8:1900\r\n") || strings.Contains(q, "MX:") || strings.Contains(q, "239.255.255.250") {
		t.Errorf("unicast M-SEARCH is wrong:\n%s", q)
	}
	if !strings.HasPrefix(q, "M-SEARCH * HTTP/1.1\r\n") || !strings.HasSuffix(q, "\r\n\r\n") {
		t.Errorf("M-SEARCH framing is wrong:\n%s", q)
	}
}

// nbstatReply builds a node status response as Windows sends it: no question
// section, one answer holding the name table and the adapter's MAC.
func nbstatReply(withQuestion bool, mac []byte, names ...[18]byte) []byte {
	encoded := append([]byte{32}, []byte("CKAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")...)
	encoded = append(encoded, 0)
	qd := uint16(0)
	if withQuestion {
		qd = 1
	}
	p := []byte{0x12, 0x34, 0x84, 0x00, byte(qd >> 8), byte(qd), 0, 1, 0, 0, 0, 0}
	if withQuestion {
		p = append(append(p, encoded...), 0, 33, 0, 1)
	}
	data := []byte{byte(len(names))}
	for _, n := range names {
		data = append(data, n[:]...)
	}
	data = append(data, mac...)
	data = append(data, make([]byte, 40)...) // statistics, ignored
	p = append(append(p, encoded...), 0, 33, 0, 1, 0, 0, 0, 0, byte(len(data)>>8), byte(len(data)))
	return append(p, data...)
}

func nbName(name string, suffix byte, flags uint16) [18]byte {
	var e [18]byte
	copy(e[:15], []byte(name + "               ")[:15])
	e[15], e[16], e[17] = suffix, byte(flags>>8), byte(flags)
	return e
}

func TestParseNetBIOSReadsAWindowsReply(t *testing.T) {
	mac := []byte{0x3c, 0x52, 0x82, 0x1a, 0x2b, 0x3c}
	names := [][18]byte{nbName("DFHO-PC-12", 0x00, 0x0400), nbName("CALF", 0x00, 0x8400), nbName("DFHO-PC-12", 0x20, 0x0400)}
	for _, echo := range []bool{false, true} {
		got, err := parseNetBIOS(nbstatReply(echo, mac, names...))
		if err != nil || got.name != "DFHO-PC-12" || got.mac != "3c:52:82:1a:2b:3c" || got.group != "CALF" {
			t.Errorf("question echoed %v: got %+v %v", echo, got, err)
		}
	}
}

// Descriptions declared as ISO-8859-1, as many printers send them, are read.
func TestFetchDescriptionReadsLatin1(t *testing.T) {
	body := "<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?>\n<root xmlns=\"urn:schemas-upnp-org:device-1-0\"><device>" +
		"<deviceType>urn:schemas-upnp-org:device:Printer:1</deviceType><friendlyName>Floor 2 printer</friendlyName>" +
		"<manufacturer>Kyocera\xae</manufacturer><modelName>ECOSYS M2540dn</modelName></device></root>"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
	defer srv.Close()
	got := fetchDescription(context.Background(), netip.MustParseAddr("127.0.0.1"), srv.URL+"/desc.xml")
	if got.Name != "Floor 2 printer" || got.Maker != "Kyocera®" || got.Model != "ECOSYS M2540dn" || got.Kind != "Printer" {
		t.Errorf("latin-1 description: %+v", got)
	}
}
