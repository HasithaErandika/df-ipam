package main

import (
	"context"
	"fmt"
	"time"

	"dfipam/api/internal/modules/integrations"
	"dfipam/api/internal/modules/subnets"
	"dfipam/api/internal/platform/safehttp"
)

// fixedConnector returns sample results, so no real platform is called.
type fixedConnector struct{ result *integrations.Result }

func (f fixedConnector) Test(context.Context) error { return nil }
func (f fixedConnector) Collect(context.Context) (*integrations.Result, error) {
	return f.result, nil
}

func fortiResult() *integrations.Result {
	now := time.Now()
	r := &integrations.Result{Devices: []integrations.Device{{ExternalID: "sample-fw", Name: "fw-cmb-01", Hostname: "fw-cmb-01", DeviceType: "firewall", Vendor: "Fortinet", Model: "FortiGate 600F", OSVersion: "Sample", OSFamily: "network_os",
		VDOMs:      []integrations.VDOM{{Name: "root", Context: "vdom:root"}, {Name: "dmz", Context: "vdom:dmz"}},
		Interfaces: []integrations.Interface{{Name: "port1", Type: "physical", AdminStatus: "up", VDOM: "root", Context: "vdom:root"}, {Name: "port2", Type: "physical", AdminStatus: "up", VDOM: "dmz", Context: "vdom:dmz"}}}},
		AddressObjects: []integrations.AddressObject{{DeviceExternalID: "sample-fw", VDOM: "root", Name: "dc-servers", Type: "ipmask", Value: "10.10.0.0/16", Network: "10.10.0.0/16"}},
		Routes:         []integrations.Route{{Context: "vdom:root", DeviceExternalID: "sample-fw", VDOM: "root", Destination: "10.100.0.0/16", NextHop: "10.255.0.1", Type: "static"}}}
	for i := 1; i <= 20; i++ {
		r.DHCPScopes = append(r.DHCPScopes, integrations.DHCPScope{Context: "vdom:root", DeviceExternalID: "sample-fw", Name: fmt.Sprintf("Branch %d users", i), RangeStart: fmt.Sprintf("10.100.%d.40", i), RangeEnd: fmt.Sprintf("10.100.%d.90", i)})
		r.Observations = append(r.Observations, integrations.Observation{Source: integrations.SourceFortiGate, Context: "vdom:root", Address: fmt.Sprintf("10.100.%d.12", i), MAC: macFor(300, i), Alive: true, ObservedAt: now})
	}
	r.Warnings = []integrations.Warning{{Code: "ARP_UNAVAILABLE", Message: "Sample warning: the token can't read one table", Context: "vdom:dmz"}}
	return r
}

func awsResult() *integrations.Result {
	r := &integrations.Result{Accounts: []integrations.CloudAccount{{Provider: "aws", ExternalID: "000000000000", Name: "Sample account"}},
		Networks: []integrations.CloudNetwork{{AccountExternalID: "000000000000", ExternalID: "vpc-sample", Name: "Sample VPC", Region: "ap-south-1", AddressSpaces: []string{"10.50.0.0/16"}}}}
	for i := 1; i <= 6; i++ {
		r.Observations = append(r.Observations, integrations.Observation{Source: integrations.SourceAWS, Context: "vpc-sample", Address: fmt.Sprintf("10.50.%d.10", i), Alive: true, ObservedAt: time.Now()})
	}
	return r
}

func (s *sample) addIntegrations() {
	forti := s.create("admin", "/integrations/connections", "", H{"kind": "fortigate", "name": "Sample FortiGate", "baseUrl": "https://fw.sample.example", "isEnabled": false,
		"credentials": H{"apiToken": "sample-token"}, "routingDomains": H{"vdom:root": s.ref["rd:corp"], "vdom:dmz": s.ref["rd:dmz"]}})
	aws := s.create("admin", "/integrations/connections", "", H{"kind": "aws", "name": "Sample AWS account", "isEnabled": false,
		"config": H{"roleArn": "arn:aws:iam::000000000000:role/sample-read-only", "externalId": "sample", "regions": "ap-south-1"}, "routingDomains": H{"vpc-sample": s.ref["rd:aws"]}})
	svc := integrations.NewService(s.pool, s.keys, s.policy).WithBuilder(func(c integrations.Connection, _ map[string]string, _ safehttp.Policy) (integrations.Connector, error) {
		if c.Kind == "aws" {
			return fixedConnector{awsResult()}, nil
		}
		return fixedConnector{fortiResult()}, nil
	})
	for _, id := range []string{forti, aws} {
		s.must("admin", "POST", "/integrations/connections/"+id+"/sync", nil)
	}
	res, err := svc.RunSyncs(s.ctx)
	if err != nil || res.Failed > 0 {
		panic(fmt.Sprintf("sample sync: %+v %v", res, err))
	}
}

func (s *sample) addFindings() {
	s.must("engineer", "POST", "/reconciliation/run", nil)
	_, out := s.call("engineer", "GET", "/reconciliation/results?pageSize=200", nil)
	counts := map[string]int{}
	proposed := 0
	for _, row := range list(out) {
		f := row.(map[string]any)
		class, _ := f["classification"].(string)
		counts[class]++
		if class == "ownership_missing" && proposed < 2 {
			proposed++
			s.propose(f["id"].(string), proposed == 1)
		} else if counts[class] == 2 {
			s.must("engineer", "POST", "/reconciliation/results/"+f["id"].(string)+"/review", H{"status": "investigating", "note": "Sample review"})
		} else if counts[class] == 3 {
			s.must("engineer", "POST", "/reconciliation/results/"+f["id"].(string)+"/review", H{"status": "dismissed", "note": "Known and accepted. Sample data."})
		}
	}
	fmt.Printf("findings: %v\n", counts)
}

// propose leaves one change waiting for a decision, and has one decided.
func (s *sample) propose(finding string, decide bool) {
	change := s.must("engineer", "POST", "/reconciliation/results/"+finding+"/change-requests", H{"changes": H{"technicalOwnerId": s.people["engineer"]}, "reason": "Assign the missing owner"})
	if decide {
		s.must("approver", "POST", "/change-requests/"+change["id"].(string)+"/approve", H{"note": "Agreed"})
	}
}

// addReports queues reports for the worker, and gives the trend charts history.
func (s *sample) addReports() {
	for _, r := range [][2]string{{"subnet_register", "xlsx"}, {"allocation_register", "csv"}, {"ownership_gaps", "pdf"}} {
		s.must("admin", "POST", "/reports", H{"reportType": r[0], "format": r[1]})
	}
	for days := 14; days >= 1; days-- {
		if _, err := subnets.SnapshotDay(s.ctx, s.pool, time.Now().AddDate(0, 0, -days)); err != nil {
			panic(err)
		}
	}
}
