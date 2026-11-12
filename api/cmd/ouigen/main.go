// Command ouigen turns the IEEE MA-L registry CSV (standard input) into the
// compact "AABBCC<TAB>Organisation" list the scanner embeds (standard output).
package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// Legal suffixes add nothing on a screen: "Samsung Electronics Co.,Ltd" reads as "Samsung Electronics".
var suffix = regexp.MustCompile(`(?i)[,.]?\s*(co\.?,?\s*ltd\.?|corporation|corp\.?|inc\.?|incorporated|limited|ltd\.?|gmbh|s\.?a\.?|llc|b\.v\.|ag|oy|ab|pte|pty)\.?$`)

func clean(name string) string {
	name = strings.Join(strings.Fields(name), " ")
	for i := 0; i < 3; i++ {
		name = strings.TrimRight(suffix.ReplaceAllString(name, ""), " ,.")
	}
	return name
}

func main() {
	r := csv.NewReader(bufio.NewReader(os.Stdin))
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		fmt.Fprintln(os.Stderr, "ouigen:", err)
		os.Exit(1)
	}
	out := map[string]string{}
	for _, row := range rows[1:] {
		if len(row) >= 3 && row[0] == "MA-L" && len(row[1]) == 6 {
			out[strings.ToUpper(row[1])] = clean(row[2])
		}
	}
	keys := make([]string, 0, len(out))
	for k := range out {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()
	for _, k := range keys {
		fmt.Fprintf(w, "%s\t%s\n", k, out[k])
	}
}
