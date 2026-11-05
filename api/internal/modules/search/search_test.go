package search

import "testing"

func TestParse(t *testing.T) {
	if q := Parse(" 10.0.0.5 "); q.Addr == nil || q.Addr.String() != "10.0.0.5" || q.Prefix != nil {
		t.Errorf("address: %+v", q)
	}
	if q := Parse("::ffff:10.0.0.5"); q.Addr == nil || q.Addr.String() != "10.0.0.5" {
		t.Errorf("mapped address: %+v", q)
	}
	if q := Parse("10.0.0.7/24"); q.Prefix == nil || q.Prefix.String() != "10.0.0.0/24" {
		t.Errorf("prefix: %+v", q)
	}
	if q := Parse("120"); q.Number == nil || *q.Number != 120 || q.Addr != nil {
		t.Errorf("vlan: %+v", q)
	}
	if q := Parse("app-server"); q.Addr != nil || q.Prefix != nil || q.Number != nil {
		t.Errorf("text: %+v", q)
	}
}

func TestFindersCoverEachKind(t *testing.T) {
	kinds := func(q Query) map[string]bool {
		out := map[string]bool{}
		for _, f := range finders(q) {
			out[f.kind] = true
		}
		return out
	}
	if k := kinds(Parse("10.0.0.5")); !k["subnet"] || !k["ip_address"] || k["site"] {
		t.Errorf("address: %v", k)
	}
	if k := kinds(Parse("core")); !k["site"] || !k["vlan"] || !k["dns_record"] {
		t.Errorf("text: %v", k)
	}
}

func TestEscapeLike(t *testing.T) {
	if got := escapeLike(`a%b_c\`); got != `a\%b\_c\\` {
		t.Error(got)
	}
}
