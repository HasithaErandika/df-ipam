package allocation

import (
	"errors"
	"net/netip"
	"testing"
	"time"

	"dfipam/api/internal/netcalc"
	"dfipam/api/internal/platform/httpx"
)

var today = time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)

func str(s string) *string { return &s }

const subnetID = "11111111-1111-1111-1111-111111111111"

func code(err error) string {
	var e *httpx.Error
	if errors.As(err, &e) {
		if len(e.Details) > 0 {
			return e.Code + ":" + e.Details[0].Field
		}
		return e.Code
	}
	if err != nil {
		return err.Error()
	}
	return ""
}

func TestNormalise(t *testing.T) {
	base := func() RequestInput {
		return RequestInput{RequestType: TypeAllocation, SubnetID: subnetID, Justification: "  new app server  "}
	}
	got, err := Normalise(base(), today)
	if err != nil || got.SelectionMode != "automatic" || got.Justification != "new app server" {
		t.Fatalf("automatic default: %+v %v", got, err)
	}

	in := base()
	in.SelectionMode, in.RequestedAddress = "specific", str(" 10.0.0.5 ")
	if got, err := Normalise(in, today); err != nil || *got.RequestedAddress != "10.0.0.5" {
		t.Fatalf("specific: %+v %v", got, err)
	}

	for name, c := range map[string]struct {
		mutate func(*RequestInput)
		want   string
	}{
		"bad type":            {func(r *RequestInput) { r.RequestType = "steal" }, "VALIDATION_FAILED:requestType"},
		"no justification":    {func(r *RequestInput) { r.Justification = " " }, "VALIDATION_FAILED:justification"},
		"no subnet":           {func(r *RequestInput) { r.SubnetID = "x" }, "VALIDATION_FAILED:subnetId"},
		"specific no address": {func(r *RequestInput) { r.SelectionMode = "specific" }, "VALIDATION_FAILED:requestedAddress"},
		"bad address": {func(r *RequestInput) { r.SelectionMode, r.RequestedAddress = "specific", str("10.0.0.300") },
			"VALIDATION_FAILED:requestedAddress"},
		"mapped v4 address": {func(r *RequestInput) { r.SelectionMode, r.RequestedAddress = "specific", str("::ffff:10.0.0.1") },
			"VALIDATION_FAILED:requestedAddress"},
		"bad mode":        {func(r *RequestInput) { r.SelectionMode = "random" }, "VALIDATION_FAILED:selectionMode"},
		"bad environment": {func(r *RequestInput) { r.Environment = str("prod") }, "VALIDATION_FAILED:environment"},
		"bad owner":       {func(r *RequestInput) { r.TechnicalOwnerID = str("bob") }, "VALIDATION_FAILED:technicalOwnerId"},
		"bad date":        {func(r *RequestInput) { r.RequiredDate = str("05/11/2026") }, "VALIDATION_FAILED:requiredDate"},
		"past expiry":     {func(r *RequestInput) { r.ExpiryDate = str("2026-11-05") }, "VALIDATION_FAILED:expiryDate"},
		"expiry before required": {func(r *RequestInput) { r.RequiredDate, r.ExpiryDate = str("2026-12-10"), str("2026-12-01") },
			"VALIDATION_FAILED:expiryDate"},
		"reservation without expiry": {func(r *RequestInput) { r.RequestType = TypeReservation }, "VALIDATION_FAILED:expiryDate"},
		"reservation over a year": {func(r *RequestInput) { r.RequestType, r.ExpiryDate = TypeReservation, str("2027-11-06") },
			"VALIDATION_FAILED:expiryDate"},
		"release without allocation": {func(r *RequestInput) { r.RequestType = TypeRelease }, "VALIDATION_FAILED:allocationId"},
	} {
		in := base()
		c.mutate(&in)
		if _, err := Normalise(in, today); code(err) != c.want {
			t.Errorf("%s: got %q want %q", name, code(err), c.want)
		}
	}

	res := base()
	res.RequestType, res.ExpiryDate = TypeReservation, str("2027-11-05")
	if _, err := Normalise(res, today); err != nil {
		t.Errorf("one-year reservation: %v", err)
	}

	rel := base()
	rel.RequestType, rel.AllocationID, rel.SubnetID = TypeRelease, str(subnetID), ""
	if got, err := Normalise(rel, today); err != nil || got.SelectionMode != "specific" {
		t.Errorf("release: %+v %v", got, err)
	}

	conv := base()
	conv.ReservationID, conv.SubnetID = str(subnetID), ""
	if got, err := Normalise(conv, today); err != nil || got.SelectionMode != "specific" || got.RequestedAddress != nil {
		t.Errorf("conversion: %+v %v", got, err)
	}
}

func TestReservationEnd(t *testing.T) {
	end, err := ReservationEnd("2026-12-31")
	if err != nil || !end.Equal(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("got %v %v", end, err)
	}
}

func TestChooseWorkflow(t *testing.T) {
	step := []Step{{StepOrder: 1, ApproverRoleID: subnetID}}
	workflows := []Workflow{
		{Name: "b-general", IsActive: true, Steps: step},
		{Name: "a-production", IsActive: true, Steps: step, MatchRules: MatchRules{Environments: []string{"production"}}},
		{Name: "sensitive-prod", IsActive: true, Steps: step,
			MatchRules: MatchRules{Environments: []string{"production"}, Sensitivities: []string{"sensitive", "restricted"}}},
		{Name: "inactive-release", IsActive: false, Steps: step, MatchRules: MatchRules{RequestTypes: []string{"release"}}},
		{Name: "no-steps", IsActive: true, MatchRules: MatchRules{RequestTypes: []string{"reservation"}}},
	}
	for _, c := range []struct {
		target Target
		want   string
	}{
		{Target{"allocation", "testing", "standard"}, "b-general"},
		{Target{"allocation", "production", "standard"}, "a-production"},
		{Target{"allocation", "production", "restricted"}, "sensitive-prod"},
		{Target{"release", "testing", "standard"}, "b-general"},
		{Target{"reservation", "uat", "standard"}, "b-general"},
	} {
		got := ChooseWorkflow(workflows, c.target)
		if got == nil || got.Name != c.want {
			t.Errorf("%+v: got %v want %s", c.target, got, c.want)
		}
	}
	if ChooseWorkflow(workflows[3:], Target{"release", "testing", "standard"}) != nil {
		t.Error("inactive or stepless workflow chosen")
	}
}

func TestValidateRules(t *testing.T) {
	if ValidateRules(MatchRules{RequestTypes: []string{"allocation"}, Environments: []string{"uat"}, Sensitivities: []string{"standard"}}) != nil {
		t.Error("valid rules rejected")
	}
	for _, bad := range []MatchRules{{RequestTypes: []string{"x"}}, {Environments: []string{"prod"}}, {Sensitivities: []string{"secret"}}} {
		if ValidateRules(bad) == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
}

func r(first, last string) netcalc.Range {
	return netcalc.Range{First: netip.MustParseAddr(first), Last: netip.MustParseAddr(last)}
}

func TestNextFree(t *testing.T) {
	for _, c := range []struct {
		prefix  string
		blocked []netcalc.Range
		want    string
	}{
		{"10.0.0.0/24", nil, "10.0.0.1"},
		{"10.0.0.0/24", []netcalc.Range{r("10.0.0.1", "10.0.0.1"), r("10.0.0.2", "10.0.0.9")}, "10.0.0.10"},
		{"10.0.0.0/24", []netcalc.Range{r("10.0.0.3", "10.0.0.3")}, "10.0.0.1"},
		{"10.0.0.0/31", nil, "10.0.0.0"},
		{"10.0.0.0/31", []netcalc.Range{r("10.0.0.0", "10.0.0.0")}, "10.0.0.1"},
		{"10.0.0.7/32", nil, "10.0.0.7"},
		{"2001:db8::/64", []netcalc.Range{r("2001:db8::", "2001:db8::ff")}, "2001:db8::100"},
		{"2001:db8::/127", []netcalc.Range{r("2001:db8::", "2001:db8::")}, "2001:db8::1"},
		// A blocked range reaching past the subnet is clipped, not an error.
		{"10.0.0.0/30", []netcalc.Range{r("9.0.0.0", "10.0.0.1")}, "10.0.0.2"},
	} {
		got, ok := NextFree(Space{Prefix: netip.MustParsePrefix(c.prefix), Blocked: c.blocked})
		if !ok || got.String() != c.want {
			t.Errorf("%s: got %v %v want %s", c.prefix, got, ok, c.want)
		}
	}
	full := Space{Prefix: netip.MustParsePrefix("10.0.0.0/30"), Blocked: []netcalc.Range{r("10.0.0.1", "10.0.0.2")}}
	if a, ok := NextFree(full); ok {
		t.Errorf("exhausted /30 returned %v", a)
	}
	// The whole of an IPv6 /64 except its last address: found without enumerating.
	huge := Space{Prefix: netip.MustParsePrefix("2001:db8::/64"), Blocked: []netcalc.Range{r("2001:db8::", "2001:db8::ffff:ffff:ffff:fffe")}}
	if a, ok := NextFree(huge); !ok || a.String() != "2001:db8::ffff:ffff:ffff:ffff" {
		t.Errorf("huge: got %v %v", a, ok)
	}
}

func TestCheckSpecific(t *testing.T) {
	s := Space{Prefix: netip.MustParsePrefix("10.0.0.0/24"), Blocked: []netcalc.Range{r("10.0.0.1", "10.0.0.1"), r("10.0.0.100", "10.0.0.150")}}
	for addr, want := range map[string]string{
		"10.0.0.5":   "",
		"10.0.0.1":   "ADDRESS_UNAVAILABLE",
		"10.0.0.120": "ADDRESS_UNAVAILABLE",
		"10.0.0.0":   "VALIDATION_FAILED:requestedAddress",
		"10.0.0.255": "VALIDATION_FAILED:requestedAddress",
		"10.0.1.5":   "ADDRESS_NOT_IN_SUBNET",
		"2001:db8::": "ADDRESS_NOT_IN_SUBNET",
	} {
		if got := code(CheckSpecific(s, netip.MustParseAddr(addr))); got != want {
			t.Errorf("%s: got %q want %q", addr, got, want)
		}
	}
}
