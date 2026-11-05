package discovery

import "testing"

func TestFitsBudget(t *testing.T) {
	for _, c := range []struct {
		running, job, ceiling int
		want                  bool
	}{
		{0, 50, 500, true},
		{450, 50, 500, true},
		{451, 50, 500, false},
		{0, 1000, 500, true}, // alone on the network, or it would never run
		{10, 1000, 500, false},
	} {
		if got := fitsBudget(c.running, c.job, c.ceiling); got != c.want {
			t.Errorf("%+v: got %v", c, got)
		}
	}
}
