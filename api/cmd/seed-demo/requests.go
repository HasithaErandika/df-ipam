package main

import (
	"fmt"
	"time"
)

func (s *sample) addWorkflows() {
	rows := list(H{"data": s.rolesList()})
	roles := map[string]string{}
	for _, r := range rows {
		m := r.(map[string]any)
		roles[m["name"].(string)], _ = m["id"].(string)
	}
	s.must("admin", "POST", "/approval-workflows", H{"name": "Sensitive networks: two steps", "matchRules": H{"sensitivities": []string{"sensitive", "restricted"}},
		"steps": []H{{"approverRoleId": roles["IPAM Administrator"]}, {"approverRoleId": roles["Super Administrator"]}}})
	s.must("admin", "POST", "/approval-workflows", H{"name": "Production reservations", "matchRules": H{"requestTypes": []string{"reservation"}, "environments": []string{"production"}},
		"steps": []H{{"approverRoleId": roles["IPAM Administrator"]}}})
}

func (s *sample) rolesList() []any {
	_, out := s.call("admin", "GET", "/roles", nil)
	return list(out)
}

func date(days int) string { return time.Now().AddDate(0, 0, days).Format("2006-01-02") }

// ask submits a request and returns its ID.
func (s *sample) ask(person, kind, subnet, why string, more H) string {
	body := H{"requestType": kind, "justification": why, "submit": true, "department": "Sample department", "applicationId": s.ref["app:erp"]}
	if subnet != "" {
		body["subnetId"] = s.ref["subnet:"+subnet]
	}
	for k, v := range more {
		body[k] = v
	}
	return s.must(person, "POST", "/requests", body)["id"].(string)
}

// decide approves or rejects every open step, each by the next person.
func (s *sample) decide(id, verdict string, people ...string) {
	for _, person := range people {
		r := s.must(person, "GET", "/requests/"+id, nil)
		step, _ := r["myApprovalId"].(string)
		s.must(person, "POST", "/approvals/"+step+"/"+verdict, H{"comment": "Reviewed. Sample data."})
	}
}

func (s *sample) allocate(subnet, why string, people []string, more H) H {
	id := s.ask("requester", "allocation", subnet, why, more)
	s.decide(id, "approve", people...)
	out := s.must("engineer", "POST", "/allocations", H{"requestId": id})
	return out
}

func (s *sample) reserve(subnet string, days int) H {
	id := s.ask("requester2", "reservation", subnet, "Held for an upcoming project", H{"expiryDate": date(days)})
	s.decide(id, "approve", "approver")
	return s.must("engineer", "POST", "/reservations", H{"requestId": id})
}

func (s *sample) addRequests() {
	var made []H
	for i := 0; i < 40; i++ {
		made = append(made, s.allocate(fmt.Sprintf("dc%d", i%12), "New server for a sample project", []string{"approver"}, nil))
	}
	for i := 0; i < 10; i++ {
		s.allocate(fmt.Sprintf("br%du", i), "New branch workstation", []string{"approver2"}, H{"selectionMode": "specific", "requestedAddress": fmt.Sprintf("10.100.%d.100", i+1)})
	}
	for i := 0; i < 6; i++ {
		s.allocate(fmt.Sprintf("core%d", i%4), "Core banking capacity", []string{"approver", "admin"}, nil)
	}
	var held []H
	for i := 0; i < 14; i++ {
		days := 60 + i
		if i < 4 {
			days = 3 + i // reminders are due for these
		}
		held = append(held, s.reserve(fmt.Sprintf("dc%d", i%8), days))
	}
	s.releases(made[:5])
	s.conversions(held[10:12])
	s.openRequests()
}

func (s *sample) releases(list []H) {
	for _, a := range list {
		allocation := a["id"].(string)
		id := s.ask("requester", "release", "", "No longer needed", H{"allocationId": allocation})
		s.decide(id, "approve", "approver")
		s.must("engineer", "POST", "/allocations/"+allocation+"/release", H{"requestId": id})
	}
}

func (s *sample) conversions(list []H) {
	for _, r := range list {
		id := s.ask("requester2", "allocation", "", "The project went ahead", H{"reservationId": r["id"]})
		s.decide(id, "approve", "approver")
		s.must("engineer", "POST", "/allocations", H{"requestId": id})
	}
}

// openRequests leaves work on the screens: these wait for you.
func (s *sample) openRequests() {
	for i := 0; i < 10; i++ {
		s.ask("requester", "allocation", fmt.Sprintf("dc%d", i), "Waiting for an approver", nil)
	}
	for i := 0; i < 6; i++ {
		s.decide(s.ask("requester2", "allocation", fmt.Sprintf("hou%d", i+1), "Approved, waiting to be fulfilled", nil), "approve", "approver")
	}
	for i := 0; i < 4; i++ {
		s.must("requester", "POST", "/requests", H{"requestType": "allocation", "subnetId": s.ref[fmt.Sprintf("subnet:br%du", i)], "justification": "Still being written"})
	}
	for i := 0; i < 5; i++ {
		s.decide(s.ask("requester", "allocation", fmt.Sprintf("dmz%d", i), "Needs more detail", nil), "reject", "approver2")
	}
	for i := 0; i < 3; i++ {
		s.must("requester2", "POST", "/requests/"+s.ask("requester2", "allocation", fmt.Sprintf("dc%d", i), "No longer needed", nil)+"/cancel", nil)
	}
}
