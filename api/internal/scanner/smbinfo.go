package scanner

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/netip"
	"time"
	"unicode/utf16"
)

// windowsInfo is what a Windows host says about itself when a connection to
// its file sharing (SMB, port 445) starts: the NTLM challenge carries its
// Windows version and build, its full DNS name and its domain. Read the way
// nmap's smb2 ntlm-info does: negotiate, then ask to sign in without
// credentials and hang up at the challenge. Nobody signs in, and nothing on
// the host changes.
type windowsInfo struct {
	fqdn, dnsDomain, domain, build string
}

const smbTimeout = 2 * time.Second

// smbInfo asks one host. A host without SMB, or that refuses, gives nothing.
func smbInfo(parent context.Context, ip netip.Addr) windowsInfo {
	ctx, cancel := context.WithTimeout(parent, smbTimeout)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp4", net.JoinHostPort(ip.String(), "445"))
	if err != nil {
		return windowsInfo{}
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(smbTimeout))
	if _, err := smbExchange(conn, smbNegotiate()); err != nil {
		return windowsInfo{}
	}
	reply, err := smbExchange(conn, smbSessionSetup())
	if err != nil {
		return windowsInfo{}
	}
	info, _ := parseChallenge(reply)
	return info
}

// smbExchange sends one SMB2 message in its 4-byte session header and reads one reply.
func smbExchange(conn net.Conn, msg []byte) ([]byte, error) {
	frame := append([]byte{0, byte(len(msg) >> 16), byte(len(msg) >> 8), byte(len(msg))}, msg...)
	if _, err := conn.Write(frame); err != nil {
		return nil, err
	}
	var head [4]byte
	if _, err := io.ReadFull(conn, head[:]); err != nil {
		return nil, err
	}
	n := int(head[1])<<16 | int(head[2])<<8 | int(head[3])
	if n < 64 || n > 64<<10 {
		return nil, fmt.Errorf("unexpected SMB reply size %d", n)
	}
	reply := make([]byte, n)
	if _, err := io.ReadFull(conn, reply); err != nil {
		return nil, err
	}
	if !bytes.HasPrefix(reply, []byte("\xfeSMB")) {
		return nil, fmt.Errorf("not an SMB2 reply")
	}
	return reply, nil
}

// smbHeader is a 64-byte SMB2 header for one command, asking for one credit.
func smbHeader(command, messageID uint16) []byte {
	h := make([]byte, 64)
	copy(h, "\xfeSMB")
	binary.LittleEndian.PutUint16(h[4:], 64)
	binary.LittleEndian.PutUint16(h[12:], command)
	binary.LittleEndian.PutUint16(h[14:], 1)
	binary.LittleEndian.PutUint64(h[24:], uint64(messageID))
	return h
}

// smbNegotiate offers SMB 2.0.2 to 3.0.2 (3.1.1 needs negotiate contexts, and
// every Windows that speaks it also speaks 3.0.2).
func smbNegotiate() []byte {
	dialects := []uint16{0x0202, 0x0210, 0x0300, 0x0302}
	body := make([]byte, 36, 36+2*len(dialects))
	binary.LittleEndian.PutUint16(body[0:], 36)
	binary.LittleEndian.PutUint16(body[2:], uint16(len(dialects)))
	binary.LittleEndian.PutUint16(body[4:], 1) // signing enabled, not required
	for _, d := range dialects {
		body = binary.LittleEndian.AppendUint16(body, d)
	}
	return append(smbHeader(0, 0), body...)
}

// ntlmNegotiate asks for the target's details (NEGOTIATE_TARGET_INFO and VERSION).
func ntlmNegotiate() []byte {
	const flags = 0x00000001 | 0x00000004 | 0x00000200 | 0x00008000 | 0x00080000 | 0x00800000 | 0x02000000 | 0x20000000 | 0x40000000 | 0x80000000
	m := append([]byte("NTLMSSP\x00"), 1, 0, 0, 0)
	m = binary.LittleEndian.AppendUint32(m, flags)
	return append(m, make([]byte, 24)...) // empty domain and workstation, zero version
}

// smbSessionSetup wraps the NTLM negotiate in a SPNEGO NegTokenInit.
func smbSessionSetup() []byte {
	ntlmOID := []byte{0x06, 0x0a, 0x2b, 0x06, 0x01, 0x04, 0x01, 0x82, 0x37, 0x02, 0x02, 0x0a}
	mechs := asn1(0xa0, asn1(0x30, ntlmOID))
	token := asn1(0xa2, asn1(0x04, ntlmNegotiate()))
	spnego := asn1(0x60, append([]byte{0x06, 0x06, 0x2b, 0x06, 0x01, 0x05, 0x05, 0x02}, asn1(0xa0, asn1(0x30, append(mechs, token...)))...))
	body := make([]byte, 24)
	binary.LittleEndian.PutUint16(body[0:], 25)
	body[3] = 1                                     // signing enabled
	binary.LittleEndian.PutUint16(body[12:], 64+24) // security buffer offset
	binary.LittleEndian.PutUint16(body[14:], uint16(len(spnego)))
	return append(append(smbHeader(1, 1), body...), spnego...)
}

// asn1 encodes one DER element; every element here is shorter than 64 KB.
func asn1(tag byte, content []byte) []byte {
	n := len(content)
	switch {
	case n < 0x80:
		return append([]byte{tag, byte(n)}, content...)
	case n < 0x100:
		return append([]byte{tag, 0x81, byte(n)}, content...)
	default:
		return append([]byte{tag, 0x82, byte(n >> 8), byte(n)}, content...)
	}
}

// parseChallenge finds the NTLM challenge in a session setup reply and reads
// the version and the target details from it.
func parseChallenge(reply []byte) (windowsInfo, error) {
	at := bytes.Index(reply, []byte("NTLMSSP\x00\x02\x00\x00\x00"))
	if at < 0 || len(reply)-at < 56 {
		return windowsInfo{}, fmt.Errorf("no NTLM challenge")
	}
	c := reply[at:]
	var info windowsInfo
	// Samba (Linux, NAS boxes, macOS file sharing) sends a placeholder version
	// with build 0, which is not Windows: only a real build number is kept.
	if build := binary.LittleEndian.Uint16(c[50:]); binary.LittleEndian.Uint32(c[20:])&0x02000000 != 0 && c[48] > 0 && build > 0 {
		info.build = fmt.Sprintf("%d.%d.%d", c[48], c[49], build)
	}
	size, offset := int(binary.LittleEndian.Uint16(c[40:])), int(binary.LittleEndian.Uint32(c[44:]))
	if offset+size > len(c) {
		return info, nil
	}
	pairs := c[offset : offset+size]
	for len(pairs) >= 4 {
		id, n := binary.LittleEndian.Uint16(pairs), int(binary.LittleEndian.Uint16(pairs[2:]))
		if id == 0 || 4+n > len(pairs) {
			break
		}
		value := clip(utf16le(pairs[4 : 4+n]))
		switch id {
		case 2:
			info.domain = value
		case 3:
			info.fqdn = value
		case 4:
			info.dnsDomain = value
		}
		pairs = pairs[4+n:]
	}
	return info, nil
}

func utf16le(b []byte) string {
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(b[2*i:])
	}
	return string(utf16.Decode(u))
}
