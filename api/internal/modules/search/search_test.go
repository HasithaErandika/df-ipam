package search

import "testing"

func TestParse(t *testing.T) {
	if _, err := Parse(" a "); err == nil {
		t.Error("one character accepted")
	}
	q, err := Parse("10.0.0.5")
	if err != nil || q.Addr == nil || q.Addr.String() != "10.0.0.5" || q.VLAN != nil {
		t.Fatalf("address: %+v %v", q, err)
	}
	if q, _ := Parse("::ffff:10.0.0.5"); q.Addr == nil || q.Addr.String() != "10.0.0.5" {
		t.Errorf("mapped address not unmapped: %+v", q.Addr)
	}
	if q, _ := Parse("120"); q.VLAN == nil || *q.VLAN != 120 || q.Addr != nil {
		t.Errorf("vlan: %+v", q)
	}
	if q, _ := Parse("5000"); q.VLAN != nil {
		t.Error("out-of-range VLAN accepted")
	}
	if q, _ := Parse("web-01"); q.Addr != nil || q.VLAN != nil || q.Text != "web-01" {
		t.Errorf("text: %+v", q)
	}
}

func TestValidateFilter(t *testing.T) {
	ok := SavedFilter{Page: "subnets", Name: " Prod ", Filters: map[string]string{"environment": "production"}}
	if err := ValidateFilter(&ok); err != nil || ok.Name != "Prod" {
		t.Fatalf("%v %q", err, ok.Name)
	}
	for _, bad := range []SavedFilter{
		{Page: "secrets", Name: "x", Filters: map[string]string{"a": "b"}},
		{Page: "subnets", Name: "", Filters: map[string]string{"a": "b"}},
		{Page: "subnets", Name: "x"},
	} {
		if ValidateFilter(&bad) == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
}
