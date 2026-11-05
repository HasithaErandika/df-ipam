package reconciliation

import "testing"

func TestScore(t *testing.T) {
	w := Weights{Reliability: 0.6, Freshness: 0.3, Agreement: 0.1, Cap: 90, FreshnessDays: 7}
	for _, c := range []struct {
		name        string
		reliability float64
		age         float64
		sources     int
		want        int
	}{
		{"fresh, trusted, one source", 100, 0, 1, 90},
		{"fresh, weak source", 40, 0, 1, 54},
		{"week old", 100, 7, 1, 60},
		{"older than the window adds nothing, and never goes negative", 40, 30, 1, 24},
		{"two sources agree", 40, 0, 2, 59},
		{"three sources reach full agreement", 40, 0, 3, 64},
		{"more sources add no more", 40, 0, 9, 64},
	} {
		if got := Score(w, c.reliability, c.age, c.sources); got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
}

// Several MACs alone must stay low, however good the evidence looks.
func TestCapHolds(t *testing.T) {
	w := Weights{Reliability: 0.4, Freshness: 0.3, Agreement: 0.3, Cap: 50, FreshnessDays: 1}
	if got := Score(w, 100, 0, 5); got != 50 {
		t.Fatalf("got %d", got)
	}
}

func TestWeightsValidate(t *testing.T) {
	good := Weights{Reliability: 0.5, Freshness: 0.25, Agreement: 0.25, Cap: 80, FreshnessDays: 7}
	if err := good.validate(); err != nil {
		t.Fatal(err)
	}
	if err := (Weights{}).validate(); err != nil {
		t.Fatal("empty weights mean the rule's fixed confidence")
	}
	for _, bad := range []Weights{
		{Reliability: 0.9, Freshness: 0.9, Cap: 80, FreshnessDays: 7},
		{Reliability: 1, Cap: 0, FreshnessDays: 7},
		{Reliability: 1, Cap: 101, FreshnessDays: 7},
		{Reliability: 1, Cap: 50},
		{Reliability: -1, Freshness: 2, Cap: 50, FreshnessDays: 7},
	} {
		if err := bad.validate(); err == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
}
