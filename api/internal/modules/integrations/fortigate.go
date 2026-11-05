package integrations

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"dfipam/api/internal/platform/safehttp"

	"golang.org/x/time/rate"
)

const (
	fortiPageSize = 1000
	fortiMaxBody  = 64 << 20
	fortiMaxPages = 500
	fortiRetries  = 3
)

// fortiGate reads the FortiOS REST API. Its only request method is get.
type fortiGate struct {
	base    *url.URL
	token   string
	client  *http.Client
	limiter *rate.Limiter
	backoff time.Duration
}

func newFortiGate(c Connection, credentials map[string]string, policy safehttp.Policy) (Connector, error) {
	base, err := policy.CheckURL(c.BaseURL)
	if err != nil {
		return nil, err
	}
	return &fortiGate{base, credentials["apiToken"], policy.Client(60 * time.Second), rate.NewLimiter(10, 10), time.Second}, nil
}

type fortiError struct {
	Path   string
	Status int
}

func (e *fortiError) Error() string {
	return fmt.Sprintf("FortiGate answered %d for %s", e.Status, e.Path)
}

type fortiResponse[T any] struct {
	Results []T    `json:"results"`
	Serial  string `json:"serial"`
	Version string `json:"version"`
}

// list reads every page of a table. Monitor endpoints ignore start and count, and
// a short page ends the loop.
func fortiList[T any](ctx context.Context, f *fortiGate, path, vdom string) ([]T, error) {
	var out []T
	for page := range fortiMaxPages {
		q := url.Values{"start": {strconv.Itoa(page * fortiPageSize)}, "count": {strconv.Itoa(fortiPageSize)}}
		if vdom != "" {
			q.Set("vdom", vdom)
		}
		body, err := f.get(ctx, path, q)
		if err != nil {
			return nil, err
		}
		var r fortiResponse[T]
		if err := json.Unmarshal(body, &r); err != nil {
			return nil, fmt.Errorf("%s: the response can't be read", path)
		}
		out = append(out, r.Results...)
		if len(r.Results) < fortiPageSize {
			return out, nil
		}
	}
	return out, fmt.Errorf("%s: more than %d pages", path, fortiMaxPages)
}

func (f *fortiGate) Test(ctx context.Context) error {
	_, err := f.get(ctx, "/api/v2/monitor/system/status", nil)
	return err
}

type fortiStatus struct {
	Serial  string `json:"serial"`
	Version string `json:"version"`
	Results struct {
		Hostname  string `json:"hostname"`
		ModelName string `json:"model_name"`
		Model     string `json:"model_number"`
	} `json:"results"`
}

type fortiHA struct {
	Results struct {
		Mode      string `json:"mode"`
		GroupName string `json:"group-name"`
	} `json:"results"`
}

type fortiInterface struct {
	Name        string `json:"name"`
	VDOM        string `json:"vdom"`
	IP          string `json:"ip"`
	Type        string `json:"type"`
	Status      string `json:"status"`
	Alias       string `json:"alias"`
	Description string `json:"description"`
	MAC         string `json:"macaddr"`
	VLAN        int    `json:"vlanid"`
	Secondary   []struct {
		IP string `json:"ip"`
	} `json:"secondaryip"`
}

type fortiAddress struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Subnet  string `json:"subnet"`
	StartIP string `json:"start-ip"`
	EndIP   string `json:"end-ip"`
	FQDN    string `json:"fqdn"`
	Comment string `json:"comment"`
}

type fortiGroup struct {
	Name    string `json:"name"`
	Comment string `json:"comment"`
	Member  []struct {
		Name string `json:"name"`
	} `json:"member"`
}

type fortiRoute struct {
	Dst     string `json:"dst"`
	Gateway string `json:"gateway"`
	Device  string `json:"device"`
}

type fortiARP struct {
	IP        string `json:"ip"`
	MAC       string `json:"mac"`
	Interface string `json:"interface"`
}

type fortiDHCPServer struct {
	ID        int    `json:"id"`
	Interface string `json:"interface"`
	IPRange   []struct {
		Start string `json:"start-ip"`
		End   string `json:"end-ip"`
	} `json:"ip-range"`
}

type fortiLease struct {
	IP        string `json:"ip"`
	MAC       string `json:"mac"`
	Hostname  string `json:"hostname"`
	Interface string `json:"interface"`
	Status    string `json:"status"`
}

// splitIPMask reads FortiOS's "10.0.0.1 255.255.255.0" form. Zero means unset.
func splitIPMask(v string) (netip.Addr, netip.Prefix, bool) {
	ip, mask, ok := strings.Cut(strings.TrimSpace(v), " ")
	if !ok {
		return netip.Addr{}, netip.Prefix{}, false
	}
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return netip.Addr{}, netip.Prefix{}, false
	}
	m := net.ParseIP(strings.TrimSpace(mask)).To4()
	if m == nil {
		return netip.Addr{}, netip.Prefix{}, false
	}
	ones, bits := net.IPMask(m).Size()
	if bits == 0 {
		return netip.Addr{}, netip.Prefix{}, false // not a contiguous mask
	}
	p, err := a.Prefix(ones)
	if err != nil {
		return netip.Addr{}, netip.Prefix{}, false
	}
	return a, p, true
}

func vdomContext(name string) string { return "vdom:" + name }

func fortiMAC(v string) string {
	if v == "00:00:00:00:00:00" {
		return ""
	}
	return v
}
