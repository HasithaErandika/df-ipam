package scanner

import (
	"context"
	"net/netip"
	"strings"

	"golang.org/x/net/dns/dnsmessage"
)

// mdnsModel asks a host that gave its mDNS name for its device-info record, the
// TXT record Apple devices (and some others) publish with their model, such as
// "MacBookPro18,1" or "iPhone14,2". One question to the host's own address.
func mdnsModel(parent context.Context, ip netip.Addr, name string) string {
	if name == "" || strings.ContainsAny(name, " \t") {
		return ""
	}
	return parseModel(mdnsAsk(parent, ip, name+"._device-info._tcp.local.", dnsmessage.TypeTXT))
}

// parseModel returns the model= value of the first TXT answer.
func parseModel(packet []byte) string {
	var p dnsmessage.Parser
	if _, err := p.Start(packet); err != nil {
		return ""
	}
	if err := p.SkipAllQuestions(); err != nil {
		return ""
	}
	for {
		h, err := p.AnswerHeader()
		if err != nil {
			return ""
		}
		if h.Type != dnsmessage.TypeTXT {
			if err := p.SkipAnswer(); err != nil {
				return ""
			}
			continue
		}
		r, err := p.TXTResource()
		if err != nil {
			return ""
		}
		if v, ok := txtModel(r.TXT); ok {
			return v
		}
	}
}

// txtModel finds the model= entry in one TXT record.
func txtModel(txt []string) (string, bool) {
	for _, kv := range txt {
		if v, ok := strings.CutPrefix(kv, "model="); ok {
			return clip(v), true
		}
	}
	return "", false
}
