package main

import (
	"fmt"
	"strings"
)

func (s *sample) addDiscovery() {
	policy := s.create("approver", "/discovery/policies", "", H{"name": "Sample: routine ping sweep", "description": "Sample data", "methods": []string{"icmp", "arp", "dns"},
		"targetAllowlist": []string{"10.0.0.0/8"}, "maxConcurrency": 8, "maxPps": 25, "timeoutMs": 1000, "retries": 1,
		"windows": []H{{"days": []int{1, 2, 3, 4, 5, 6, 7}, "start": "00:00", "end": "23:59"}}})
	s.must("approver2", "POST", "/discovery/policies/"+policy+"/approve", nil)
	s.create("approver", "/discovery/policies", "", H{"name": "Sample: awaiting approval", "methods": []string{"icmp"}, "targetAllowlist": []string{"10.30.0.0/16"}})
	s.create("approver", "/discovery/exclusions", "", H{"target": "10.200.0.0/16", "routingDomainId": s.ref["rd:core"], "reason": "Core banking stays passive"})
	s.create("approver", "/discovery/exclusions", "", H{"target": "10.210.0.0/16", "routingDomainId": s.ref["rd:pay"], "reason": "Payments stay passive"})
	s.create("approver", "/discovery/blackouts", "", H{"name": "Sample month-end freeze", "startsAt": date(20) + "T18:00:00Z", "endsAt": date(22) + "T02:00:00Z", "reason": "Month-end processing"})
	for _, k := range []string{"dc0", "dc1", "hou1"} {
		s.create("approver", "/discovery/schedules", "", H{"policyId": policy, "subnetId": s.ref["subnet:"+k], "cron": "30 23 * * 1-5", "timezone": "Asia/Colombo", "isEnabled": false})
	}
	agent, header := s.agent("Sample agent Colombo")
	s.agent("Sample agent Kandy")
	_, _ = s.agent("Sample agent retired")
	var ids []string
	for _, n := range s.nets {
		if n.Discover {
			ids = append(ids, n.ID)
		}
	}
	job := s.create("engineer", "/discovery/jobs", "", H{"policyId": policy, "subnetIds": ids, "agentId": agent, "priority": 1})
	s.sweep(job, header)
	failed := s.create("engineer", "/discovery/jobs", "", H{"policyId": policy, "subnetIds": ids[:1], "agentId": agent, "priority": 1})
	s.must("", "GET", "/agent-api/jobs", nil, header...)
	s.must("", "POST", "/agent-api/jobs/"+failed+"/results", H{"done": true, "error": "Sample failure: the agent host lost its route"}, header...)
	cancelled := s.create("engineer", "/discovery/jobs", "", H{"policyId": policy, "subnetIds": ids[:1], "agentId": agent})
	s.must("engineer", "POST", "/discovery/jobs/"+cancelled+"/cancel", nil)
}

func (s *sample) agent(name string) (string, []string) {
	out := s.must("approver", "POST", "/agents", H{"name": name})
	reg := s.must("", "POST", "/agent-api/register", H{"token": out["registrationToken"], "version": "sample"})
	id := reg["agentId"].(string)
	if strings.Contains(name, "retired") {
		s.must("approver", "POST", "/agents/"+id+"/revoke", nil)
	}
	return id, []string{"Authorization", "Agent " + reg["credential"].(string)}
}

// sweep plays the agent's part with sample evidence. No packet is sent.
func (s *sample) sweep(job string, header []string) {
	claim := s.must("", "GET", "/agent-api/jobs", nil, header...)
	targets, _ := claim["targets"].([]any)
	var obs []H
	for i, n := range s.nets {
		if n.Discover {
			obs = append(obs, s.evidence(i, n)...)
		}
	}
	updates := make([]H, len(targets))
	for i, t := range targets {
		updates[i] = H{"id": t.(map[string]any)["id"], "status": "done"}
	}
	s.must("", "POST", "/agent-api/jobs/"+job+"/results", H{"observations": obs, "targets": updates, "progress": 100, "done": true,
		"logs": []H{{"level": "info", "message": "Sample sweep finished"}}}, header...)
}

// evidence sees most records, misses the rest, and finds some addresses with no record.
func (s *sample) evidence(i int, n network) []H {
	var out []H
	for k := 0; k < n.Size; k++ {
		if k%5 != 4 {
			out = append(out, H{"method": "icmp", "address": fmt.Sprintf("%s%d", n.Prefix, n.Start+k), "alive": true, "mac": macFor(i, k), "hostname": fmt.Sprintf("seen-%d-%d", i, k)})
		}
	}
	for k := 0; k < 3; k++ {
		out = append(out, H{"method": "arp", "address": fmt.Sprintf("%s%d", n.Prefix, n.Start+60+k), "alive": true, "mac": macFor(200+i, k)})
	}
	return append(out, H{"method": "arp", "address": fmt.Sprintf("%s%d", n.Prefix, n.Start+70), "alive": true, "mac": macFor(0, 0)})
}
