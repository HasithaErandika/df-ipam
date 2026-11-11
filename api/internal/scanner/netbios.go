package scanner

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"time"
)

func netbiosStatus(parent context.Context, ip netip.Addr) (mac, hostname string, err error) {
	ctx, cancel := context.WithTimeout(parent, 650*time.Millisecond)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "udp4", net.JoinHostPort(ip.String(), "137"))
	if err != nil {
		return "", "", err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(650 * time.Millisecond))
	packet := netbiosQuery()
	if _, err = conn.Write(packet); err != nil {
		return "", "", err
	}
	response := make([]byte, 1500)
	n, err := conn.Read(response)
	if err != nil {
		return "", "", err
	}
	return parseNetBIOS(response[:n])
}

func netbiosQuery() []byte {
	packet := make([]byte, 12)
	binary.BigEndian.PutUint16(packet[0:2], uint16(time.Now().UnixNano()))
	binary.BigEndian.PutUint16(packet[2:4], 0x0010) // Broadcast flag, standard query.
	binary.BigEndian.PutUint16(packet[4:6], 1)
	name := make([]byte, 16)
	name[0] = '*'
	for i := 1; i < 15; i++ {
		name[i] = ' '
	}
	name[15] = 0
	encoded := make([]byte, 34)
	encoded[0] = 32
	for i, b := range name {
		encoded[1+2*i], encoded[2+2*i] = 'A'+b>>4, 'A'+b&0x0f
	}
	encoded[33] = 0
	packet = append(packet, encoded...)
	packet = append(packet, 0, 33, 0, 1) // NBSTAT, IN.
	return packet
}

func parseNetBIOS(packet []byte) (string, string, error) {
	if len(packet) < 12 || binary.BigEndian.Uint16(packet[6:8]) == 0 {
		return "", "", fmt.Errorf("NetBIOS status response has no answer")
	}
	offset := 12
	var err error
	if offset, err = skipDNSName(packet, offset); err != nil || offset+4 > len(packet) {
		return "", "", fmt.Errorf("invalid NetBIOS question")
	}
	offset += 4
	if offset, err = skipDNSName(packet, offset); err != nil || offset+10 > len(packet) {
		return "", "", fmt.Errorf("invalid NetBIOS answer")
	}
	rdataLen := int(binary.BigEndian.Uint16(packet[offset+8 : offset+10]))
	offset += 10
	if rdataLen < 1 || offset+rdataLen > len(packet) {
		return "", "", fmt.Errorf("invalid NetBIOS data length")
	}
	data := packet[offset : offset+rdataLen]
	count := int(data[0])
	if len(data) < 1+count*18+6 {
		return "", "", fmt.Errorf("truncated NetBIOS status data")
	}
	host := ""
	for i := 0; i < count; i++ {
		entry := data[1+i*18 : 1+(i+1)*18]
		flags := binary.BigEndian.Uint16(entry[16:18])
		if flags&0x8000 == 0 {
			host = strings.TrimSpace(strings.TrimRight(string(entry[:15]), "\x00"))
			if host != "" {
				break
			}
		}
	}
	macStart := 1 + count*18
	macBytes := data[macStart : macStart+6]
	if macBytes[0]|macBytes[1]|macBytes[2]|macBytes[3]|macBytes[4]|macBytes[5] == 0 {
		return "", host, nil
	}
	parts := make([]string, 6)
	for i, b := range macBytes {
		parts[i] = hex.EncodeToString([]byte{b})
	}
	return strings.Join(parts, ":"), host, nil
}

func skipDNSName(packet []byte, offset int) (int, error) {
	for offset < len(packet) {
		length := int(packet[offset])
		if length&0xc0 == 0xc0 {
			if offset+2 > len(packet) {
				return 0, fmt.Errorf("truncated compressed DNS name")
			}
			return offset + 2, nil
		}
		offset++
		if length == 0 {
			return offset, nil
		}
		if offset+length > len(packet) {
			return 0, fmt.Errorf("truncated DNS name")
		}
		offset += length
	}
	return 0, fmt.Errorf("unterminated DNS name")
}
