package reports

import (
	"sort"
	"strings"
)

// wrap breaks text into lines no wider than w: at spaces, and inside a word
// (after a separator such as - / . : when it can) only when the word alone is
// too wide. Nothing is cut off. Text is in the PDF's single-byte encoding.
func wrap(width func(string) float64, text string, w float64) []string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(text) {
		next := word
		if line != "" {
			next = line + " " + word
		}
		if width(next) <= w {
			line = next
			continue
		}
		if line != "" {
			lines = append(lines, line)
		}
		for width(word) > w { // a word wider than the column goes over several lines
			cut := fitBytes(width, word, w)
			lines = append(lines, word[:cut])
			word = word[cut:]
		}
		line = word
	}
	if line != "" || len(lines) == 0 {
		lines = append(lines, line)
	}
	return lines
}

// fitBytes is how much of word fits in w, ending after a separator if one is
// in the second half, so an address or path breaks where people expect.
func fitBytes(width func(string) float64, word string, w float64) int {
	n := 1
	for n < len(word) && width(word[:n+1]) <= w {
		n++
	}
	if i := strings.LastIndexAny(word[:n], "-/.:_,"); i >= n/2 && i+1 < n {
		return i + 1
	}
	return n
}

func longestWord(width func(string) float64, text string) float64 {
	best := 0.0
	for _, w := range strings.Fields(text) {
		best = max(best, width(w))
	}
	return best
}

func p95(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	sort.Float64s(v)
	return v[len(v)*95/100]
}
