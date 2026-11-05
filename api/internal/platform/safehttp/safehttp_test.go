package safehttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

type fixedResolver map[string][]string

func (f fixedResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	var out []netip.Addr
	for _, a := range f[host] {
		out = append(out, netip.MustParseAddr(a))
	}
	return out, nil
}

func policy() Policy {
	return Policy{
		Hosts:         []string{"fw.example.com", ".corp.example.com"},
		PrivateRanges: []netip.Prefix{netip.MustParsePrefix("10.20.0.0/16")},
	}
}

func TestCheckURL(t *testing.T) {
	p := policy()
	for raw, ok := range map[string]bool{
		"https://fw.example.com":              true,
		"https://FW.example.com:8443/api":     true,
		"https://vc.corp.example.com":         true,
		"https://corp.example.com":            false, // the suffix needs a subdomain
		"http://fw.example.com":               false,
		"https://evil.com":                    false,
		"https://fw.example.com.evil.com":     false,
		"https://evilfw.example.com":          false,
		"https://user:pass@fw.example.com":    false,
		"https://fw.example.com@evil.com":     false,
		"file:///etc/passwd":                  false,
		"gopher://fw.example.com":             false,
		"https://":                            false,
		"https://169.254.169.254/latest/meta": false,
		"https://2852039166/latest/meta-data": false, // decimal form of the metadata address
		"https://0xa9.0xfe.0xa9.0xfe/":        false,
		"https://[::ffff:169.254.169.254]/":   false,
	} {
		if _, err := p.CheckURL(raw); (err == nil) != ok {
			t.Errorf("%s: got %v", raw, err)
		}
	}
}

func TestCheckAddr(t *testing.T) {
	p := policy()
	for addr, ok := range map[string]bool{
		"10.20.1.1":              true, // private and on the allowlist
		"8.8.8.8":                true,
		"2001:4860:4860::8888":   true,
		"10.21.1.1":              false, // private, not on the allowlist
		"192.168.1.1":            false,
		"172.16.0.1":             false,
		"fd12::1":                false,
		"127.0.0.1":              false,
		"::1":                    false,
		"0.0.0.0":                false,
		"169.254.169.254":        false,
		"::ffff:169.254.169.254": false,
		"::ffff:127.0.0.1":       false,
		"::ffff:10.21.1.1":       false,
		"100.100.100.200":        false,
		"fe80::1":                false,
		"fd00:ec2::254":          false,
		"64:ff9b::a9fe:a9fe":     false,
		"2002:a9fe:a9fe::1":      false,
		"224.0.0.1":              false,
	} {
		if err := p.CheckAddr(netip.MustParseAddr(addr)); (err == nil) != ok {
			t.Errorf("%s: got %v", addr, err)
		}
	}
}

// The allowlist can't open the addresses that are never reachable.
func TestAllowlistCannotOpenLoopbackOrMetadata(t *testing.T) {
	p := Policy{PrivateRanges: []netip.Prefix{netip.MustParsePrefix("0.0.0.0/0"), netip.MustParsePrefix("::/0")}}
	for _, addr := range []string{"127.0.0.1", "169.254.169.254", "::1"} {
		if err := p.CheckAddr(netip.MustParseAddr(addr)); !errors.Is(err, ErrBlocked) {
			t.Errorf("%s: got %v", addr, err)
		}
	}
}

// An allowlisted name that resolves to a blocked address is refused at connect time,
// which is what stops DNS rebinding.
func TestDialChecksResolvedAddress(t *testing.T) {
	p := policy()
	for name, answers := range map[string][]string{
		"rebind":     {"169.254.169.254"},
		"loopback":   {"127.0.0.1"},
		"mapped":     {"::ffff:127.0.0.1"},
		"mixed":      {"8.8.8.8", "127.0.0.1"},
		"privateout": {"192.168.1.10"},
	} {
		p.Resolver = fixedResolver{"fw.example.com": answers}
		_, err := p.DialContext(context.Background(), "tcp", "fw.example.com:443")
		if !errors.Is(err, ErrBlocked) {
			t.Errorf("%s: got %v", name, err)
		}
	}
	if _, err := p.DialContext(context.Background(), "tcp", "evil.com:443"); !errors.Is(err, ErrBlocked) {
		t.Errorf("host off the allowlist: got %v", err)
	}
}

func TestRedirectsAreNotFollowed(t *testing.T) {
	var reached bool
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }))
	defer target.Close()
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer first.Close()

	client := policy().Client(time.Second)
	client.Transport = http.DefaultTransport // reach the local server; only the redirect rule is under test
	_, err := client.Get(first.URL)
	if !errors.Is(err, ErrBlocked) || reached {
		t.Fatalf("reached %v, error %v", reached, err)
	}
}

func TestTLSVerificationStaysOn(t *testing.T) {
	tr := policy().Transport()
	if tr.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("certificate verification is off")
	}
	if tr.Proxy != nil {
		t.Fatal("a proxy could bypass the address checks")
	}
}

func TestFromEnv(t *testing.T) {
	t.Setenv("INTEGRATION_ALLOWED_HOSTS", " FW.example.com , .corp.example.com ")
	t.Setenv("INTEGRATION_ALLOWED_PRIVATE_RANGES", "10.20.0.0/16")
	t.Setenv("INTEGRATION_CA_FILE", "")
	p, err := FromEnv()
	if err != nil || strings.Join(p.Hosts, ",") != "fw.example.com,.corp.example.com" || len(p.PrivateRanges) != 1 {
		t.Fatalf("%+v %v", p, err)
	}
	t.Setenv("INTEGRATION_ALLOWED_PRIVATE_RANGES", "not-a-cidr")
	if _, err := FromEnv(); err == nil {
		t.Fatal("bad range accepted")
	}
}
