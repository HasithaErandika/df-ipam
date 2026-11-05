package main

import (
	"fmt"
	"strings"
)

func (s *sample) addImports() {
	var lines []string
	for i := 1; i <= 15; i++ {
		lines = append(lines, fmt.Sprintf("10.110.%d.0/24,gw=10.110.%d.1,name=Imported network %d", i, i, i))
	}
	batch := s.upload("sample-networks.csv", strings.Join(lines, "\n"))
	s.must("approver", "POST", "/imports/"+batch+"/approve", nil)
	s.upload("sample-needs-review.csv", strings.Join([]string{"# Lines for the review screens", "10.111.1.0/24,gw=10.111.1.1,name=Ready to import", "10.111.2.0/24", "10.111.3.5/24,name=Host bits set",
		"10.10.10.0/24,name=Already in the inventory", "10.10.11.128/25,name=Overlaps a subnet", "10.111.1.0/24,name=Same as line 2", "not a network"}, "\n"))
}

func (s *sample) upload(name, content string) string {
	return s.create("engineer", "/imports", "", H{"fileName": name, "format": "csv", "content": content, "routingDomainId": s.ref["rd:corp"], "environment": "production"})
}
