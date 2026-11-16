package admin

import (
	"strconv"
	"testing"

	"dfipam/api/internal/seed"
)

// Every setting people can change has a seeded starting value inside its limits,
// so a fresh install and the Settings screen agree.
func TestEverySettingHasASeededDefault(t *testing.T) {
	for _, n := range numbers {
		v, err := strconv.Atoi(seed.Default(n.Key))
		if err != nil {
			t.Errorf("%s has no seeded default", n.Key)
			continue
		}
		if v < n.Min || v > n.Max {
			t.Errorf("%s seeds %d, outside %d to %d", n.Key, v, n.Min, n.Max)
		}
	}
}
