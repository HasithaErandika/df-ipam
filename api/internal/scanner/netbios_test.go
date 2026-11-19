package scanner

import (
	"bytes"
	"encoding/binary"
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
