package subnets

import (
	"strings"
	"testing"
)

func TestSearchClause(t *testing.T) {
	for _, tc := range []struct{ query, want, value string }{
		{"10.20.0.0/16", "<<=", "10.20.0.0/16"},
		{"10.20.5.9/16", "<<=", "10.20.0.0/16"},
		{"2001:db8::/32", "<<=", "2001:db8::/32"},
		{"10.20.1.5", ">>=", "10.20.1.5"},
		{"10.20", "ILIKE", "10.20"},
		{"Branch LAN", "ILIKE", "Branch LAN"},
	} {
		clause, value := searchClause(tc.query)
		if !strings.Contains(clause, tc.want) || value != tc.value {
			t.Errorf("%q: %s with %q", tc.query, clause, value)
		}
	}
}
