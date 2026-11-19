package scanner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/textproto"
	"net/url"
	"strings"
	"time"
)

// upnpInfo is what a device says about itself over SSDP and UPnP.
type upnpInfo struct {
	Name, Maker, Model, Kind string
}

// Describe gives the info as one line for the sightings vendor field.
func (u upnpInfo) Describe() string {
	parts := []string{}
	for _, p := range []string{u.Maker, u.Model, u.Kind} {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " · ")
}

// ssdpQuery is a unicast M-SEARCH (UPnP Device Architecture 1.1, 1.3.3): HOST
// names the device itself, and there is no MX, which only multicast searches
// use. Some devices ignore a unicast search written as a multicast one.
func ssdpQuery(ip netip.Addr) []byte {
	return []byte("M-SEARCH * HTTP/1.1\r\nHOST: " + ip.String() + ":1900\r\nMAN: \"ssdp:discover\"\r\nST: upnp:rootdevice\r\n\r\n")
}

// ssdpDescribe asks one host to describe itself: a unicast SSDP search to UDP
// 1900, then a GET of the description it points to, on the same address only.
// TVs, media players, printers and routers answer; most PCs and phones don't.
func ssdpDescribe(parent context.Context, ip netip.Addr) upnpInfo {
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	location := ssdpLocation(ctx, ip)
	if location == "" {
		return upnpInfo{}
	}
	return fetchDescription(ctx, ip, location)
}

func ssdpLocation(ctx context.Context, ip netip.Addr) string {
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return ""
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(900 * time.Millisecond))
	if _, err := conn.WriteTo(ssdpQuery(ip), &net.UDPAddr{IP: ip.AsSlice(), Port: 1900}); err != nil {
		return ""
	}
	buf := make([]byte, 2048)
	for ctx.Err() == nil {
		n, from, err := conn.ReadFrom(buf)
		if err != nil {
			return ""
		}
		if udp, ok := from.(*net.UDPAddr); !ok || !udp.IP.Equal(ip.AsSlice()) {
			continue // only the host we asked
		}
		r := textproto.NewReader(bufio.NewReader(bytes.NewReader(buf[:n])))
		if _, err := r.ReadLine(); err != nil {
			return ""
		}
		h, _ := r.ReadMIMEHeader()
		return h.Get("Location")
	}
	return ""
}

type description struct {
	Device struct {
		FriendlyName string `xml:"friendlyName"`
		Manufacturer string `xml:"manufacturer"`
		ModelName    string `xml:"modelName"`
		DeviceType   string `xml:"deviceType"`
	} `xml:"device"`
}

// fetchDescription reads the UPnP description, refusing any address but the
// host's own, so a device can't point the scanner elsewhere.
func fetchDescription(ctx context.Context, ip netip.Addr, location string) upnpInfo {
	u, err := url.Parse(location)
	if err != nil || u.Scheme != "http" || u.Hostname() != ip.String() {
		return upnpInfo{}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return upnpInfo{}
	}
	client := &http.Client{Timeout: 1500 * time.Millisecond, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		return upnpInfo{}
	}
	defer res.Body.Close()
	var d description
	dec := xml.NewDecoder(io.LimitReader(res.Body, 64<<10))
	dec.CharsetReader = latin1Reader
	if res.StatusCode != http.StatusOK || dec.Decode(&d) != nil {
		return upnpInfo{}
	}
	return upnpInfo{Name: clip(d.Device.FriendlyName), Maker: clip(d.Device.Manufacturer), Model: clip(d.Device.ModelName), Kind: deviceKind(d.Device.DeviceType)}
}

// latin1Reader lets descriptions declared as ISO-8859-1 or ASCII be read, as
// many printers and routers send them; Go's XML decoder refuses any encoding
// but UTF-8 without one. Latin-1 bytes are widened to UTF-8.
func latin1Reader(label string, input io.Reader) (io.Reader, error) {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "iso-8859-1", "iso8859-1", "latin1", "latin-1", "us-ascii", "ascii", "windows-1252":
		raw, err := io.ReadAll(input)
		if err != nil {
			return nil, err
		}
		runes := make([]rune, len(raw))
		for i, b := range raw {
			runes[i] = rune(b)
		}
		return strings.NewReader(string(runes)), nil
	}
	return nil, fmt.Errorf("unsupported encoding %q", label)
}

// deviceKind turns a UPnP device type URN into a plain word.
func deviceKind(urn string) string {
	switch {
	case strings.Contains(urn, "MediaRenderer"):
		return "TV or media player"
	case strings.Contains(urn, "MediaServer"):
		return "Media server"
	case strings.Contains(urn, "Printer"):
		return "Printer"
	case strings.Contains(urn, "InternetGatewayDevice"), strings.Contains(urn, "WANDevice"):
		return "Router"
	case strings.Contains(urn, "DigitalSecurityCamera"):
		return "Camera"
	}
	return ""
}

func clip(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 80 {
		s = s[:80]
	}
	return s
}
