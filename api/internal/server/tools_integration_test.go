package server

import (
	"fmt"
	"testing"

	"github.com/gin-gonic/gin"
)

// checkPlanningTools runs the free-space and subnet calculators.
func checkPlanningTools(t *testing.T, c client, block string) {
	t.Helper()
	space := data(c.must(200, "GET", "/network-blocks/"+block+"/free-space", nil))
	if space["freeAddresses"] != "64768" || space["largestFreeCidr"] != "10.20.128.0/17" {
		t.Fatalf("block: %v", space)
	}
	located := c.must(200, "GET", "/tools/locate?address=10.20.30.10", nil)["data"].([]any)
	if len(located) != 2 {
		t.Fatalf("locate should find both routing domains: %v", located)
	}
	if got := data(c.must(200, "POST", "/tools/split", gin.H{"cidr": "10.0.0.0/24", "newPrefix": 26}))["total"]; got != "4" {
		t.Fatal(got)
	}
	if got := fmt.Sprint(data(c.must(200, "POST", "/tools/summarise", gin.H{"cidrs": []string{"10.0.0.0/25", "10.0.0.128/25"}}))["summary"]); got != "[10.0.0.0/24]" {
		t.Fatal(got)
	}
}
