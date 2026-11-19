package scanner

import (
	"bytes"
	"encoding/binary"
	"testing"
	"unicode/utf16"
)

func avPair(id uint16, s string) []byte {
	u := utf16.Encode([]rune(s))
	b := binary.LittleEndian.AppendUint16(nil, id)
	b = binary.LittleEndian.AppendUint16(b, uint16(2*len(u)))
	for _, r := range u {
		b = binary.LittleEndian.AppendUint16(b, r)
	}
	return b
}

// challenge builds an NTLM CHALLENGE as Windows 11 23H2 sends it, inside some
// leading bytes that stand in for the SMB2 header and SPNEGO wrapping.
func challenge() []byte {
	info := append(append(append(avPair(2, "DFL"), avPair(3, "DFHO-PC-12.dialogfinance.lk")...), avPair(4, "dialogfinance.lk")...), 0, 0, 0, 0)
	c := make([]byte, 56)
	copy(c, "NTLMSSP\x00\x02\x00\x00\x00")
	binary.LittleEndian.PutUint32(c[20:], 0x02000000|0x00800000)
	binary.LittleEndian.PutUint16(c[40:], uint16(len(info)))
	binary.LittleEndian.PutUint32(c[44:], 56)
	c[48], c[49] = 10, 0
	binary.LittleEndian.PutUint16(c[50:], 22631)
	return append(append(bytes.Repeat([]byte{0xAA}, 90), c...), info...)
}

func TestParseChallenge(t *testing.T) {
	got, err := parseChallenge(challenge())
	if err != nil {
		t.Fatal(err)
	}
	want := windowsInfo{fqdn: "DFHO-PC-12.dialogfinance.lk", dnsDomain: "dialogfinance.lk", domain: "DFL", build: "10.0.22631"}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if _, err := parseChallenge([]byte("\xfeSMB nothing here")); err == nil {
		t.Error("a reply without a challenge must fail")
	}
	samba := challenge()
	samba[90+48], samba[90+49], samba[90+50], samba[90+51] = 6, 1, 0, 0
	if got, _ := parseChallenge(samba); got.build != "" || got.fqdn == "" {
		t.Errorf("a Samba placeholder version must not read as Windows: %+v", got)
	}
	short := challenge()[:90+56+10] // the whole challenge, part of the target list
	if _, err := parseChallenge(short); err != nil {
		t.Errorf("a cut-off target list keeps the version: %v", err)
	}
}

// The session setup's security buffer starts where its offset says.
func TestSessionSetupLayout(t *testing.T) {
	m := smbSessionSetup()
	offset, size := binary.LittleEndian.Uint16(m[64+12:]), binary.LittleEndian.Uint16(m[64+14:])
	if int(offset)+int(size) != len(m) || m[offset] != 0x60 {
		t.Fatalf("security buffer at %d, %d bytes, message %d bytes", offset, size, len(m))
	}
	if !bytes.Contains(m, []byte("NTLMSSP\x00\x01")) {
		t.Fatal("no NTLM negotiate in the session setup")
	}
}

func TestGroupName(t *testing.T) {
	var table []byte
	for _, e := range [][18]byte{nbName("PC-042", 0x00, 0x0400), nbName("WORKGROUP", 0x1e, 0x8400), nbName("WORKGROUP", 0x00, 0x8400)} {
		table = append(table, e[:]...)
	}
	if got := groupName(table, 3); got != "WORKGROUP" {
		t.Errorf("got %q, want WORKGROUP", got)
	}
}
