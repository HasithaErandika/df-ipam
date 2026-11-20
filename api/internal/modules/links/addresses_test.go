package links

import (
	"testing"

	"dfipam/api/internal/validate"
)

// The forms below are how the site network diagrams write addresses.
func TestHostAddressTakesDiagramForms(t *testing.T) {
	cases := map[string]string{
		"10.10.40.253 /24":              "10.10.40.253",
		"172.22.60.146/29":              "172.22.60.146",
		"122.255.4.226 255.255.255.248": "122.255.4.226",
		" 123.231.92.125 ":              "123.231.92.125",
		"122.255.36.14 255.255.255.255": "122.255.36.14",
	}
	for in, want := range cases {
		var v validate.V
		got := in
		if hostAddress(&v, "f", &got, "x"); got != want || !v.OK() {
			t.Errorf("%q: got %q, ok %v", in, got, v.OK())
		}
	}
	for _, in := range []string{"172,22,60.145", "10.0.0.1/33", "10.0.0.1 255.0.255.0", "10.0.0.1/", "fd00::1"} {
		var v validate.V
		got := in
		if hostAddress(&v, "f", &got, "x"); v.OK() {
			t.Errorf("%q was accepted as %q", in, got)
		}
	}
}

func TestIPOrNetworkTakesMasks(t *testing.T) {
	cases := map[string]string{"122.255.4.224 255.255.255.248": "122.255.4.224/29", "122.255.4.224 /29": "122.255.4.224/29", "122.255.36.14 255.255.255.255": "122.255.36.14/32"}
	for in, want := range cases {
		var v validate.V
		if got := ipOrNetwork(&v, "f", in, "x"); got != want || !v.OK() {
			t.Errorf("%q: got %q", in, got)
		}
	}
}

func TestWANNetwork(t *testing.T) {
	for in, want := range map[string]string{"10.10.40.253 /24": "10.10.40.0/24", "122.255.36.14 255.255.255.255": "122.255.36.14/32", "123.231.92.125": "", "x": ""} {
		if got := wanNetwork(in); got != want {
			t.Errorf("%q: got %q", in, got)
		}
	}
}
