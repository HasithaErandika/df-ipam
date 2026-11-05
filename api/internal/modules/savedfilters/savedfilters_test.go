package savedfilters

import "testing"

func TestValidate(t *testing.T) {
	ok := Filter{Page: "subnets", Name: " Prod ", Filters: map[string]string{"environment": "production"}}
	if err := Validate(&ok); err != nil || ok.Name != "Prod" {
		t.Fatalf("%v %q", err, ok.Name)
	}
	for _, bad := range []Filter{
		{Page: "users", Name: "x", Filters: map[string]string{"a": "b"}},
		{Page: "subnets", Name: "", Filters: map[string]string{"a": "b"}},
		{Page: "subnets", Name: "x"},
		{Page: "subnets", Name: "x", Filters: map[string]string{"page": "2"}},
	} {
		if err := Validate(&bad); err == nil {
			t.Errorf("%+v: expected rejection", bad)
		}
	}
}
