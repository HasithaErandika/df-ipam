package scanner

import (
	"bytes"
	"encoding/binary"
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
