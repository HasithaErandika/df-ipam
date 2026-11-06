package server

import (
	"testing"

	"github.com/gin-gonic/gin"
)

func total(t *testing.T, c client, path string) float64 {
	t.Helper()
	return c.must(200, "GET", path, nil)["meta"].(map[string]any)["total"].(float64)
}

func TestSearchByNetwork(t *testing.T) {
	c := setup(t)
	org := id(c.must(201, "POST", "/organisations", gin.H{"name": "Search Org", "code": "SRCH"}))
	rd := id(c.must(201, "POST", "/routing-domains", gin.H{"organisationId": org, "name": "Search VRF", "kind": "vrf"}))
	add := func(cidr string) string {
		return id(c.must(201, "POST", "/subnets", gin.H{"routingDomainId": rd, "cidr": cidr, "name": "Net " + cidr, "environment": "production"}))
	}
	add("10.91.1.0/24")
	add("10.91.2.0/24")
	add("10.91.200.0/24")
	add("10.92.1.0/24")
	c.must(201, "POST", "/ip-addresses", gin.H{"routingDomainId": rd, "address": "10.91.1.5"})
	c.must(201, "POST", "/ip-addresses", gin.H{"routingDomainId": rd, "address": "10.92.1.5"})

	for path, want := range map[string]float64{
		"/subnets?routingDomainId=" + rd + "&q=10.91.0.0/16":      3,
		"/subnets?routingDomainId=" + rd + "&q=10.91.1.0/25":      1, // the subnet around it
		"/subnets?routingDomainId=" + rd + "&q=10.91.2.7":         1,
		"/subnets?routingDomainId=" + rd + "&q=10.50.0.0/16":      0,
		"/subnets?routingDomainId=" + rd + "&q=Net":               4,
		"/ip-addresses?routingDomainId=" + rd + "&q=10.91.0.0/16": 1,
	} {
		if got := total(t, c, path); got != want {
			t.Errorf("%s: %v, want %v", path, got, want)
		}
	}
}
