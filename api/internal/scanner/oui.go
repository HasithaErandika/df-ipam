package scanner

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"strings"
	"sync"
)

// oui.txt.gz is the IEEE MA-L registry (https://standards-oui.ieee.org/oui/oui.csv)
// reduced to "AABBCC<TAB>Organisation" lines. Regenerate it with
// `go run ./cmd/ouigen < oui.csv | gzip -9 > internal/scanner/oui.txt.gz`.
//
//go:embed oui.txt.gz
var ouiData []byte

var (
	ouiOnce  sync.Once
	ouiNames map[string]string
)

func loadOUI() {
	ouiNames = map[string]string{}
	r, err := gzip.NewReader(bytes.NewReader(ouiData))
	if err != nil {
		return
	}
	defer r.Close()
	s := bufio.NewScanner(r)
	for s.Scan() {
		if prefix, name, ok := strings.Cut(s.Text(), "\t"); ok {
			ouiNames[prefix] = name
		}
	}
}

// macVendor names the maker a MAC address was assigned to, from its first three
// bytes. Randomised (private) MACs belong to no maker, so they return "".
func macVendor(mac string) string {
	if len(mac) < 8 || isPrivateMAC(mac) {
		return ""
	}
	ouiOnce.Do(loadOUI)
	return ouiNames[strings.ToUpper(strings.ReplaceAll(mac[:8], ":", ""))]
}
