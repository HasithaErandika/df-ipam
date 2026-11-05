package server

import (
	"context"
	"testing"
	"time"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/modules/discovery"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// discoverySite is a domain with two subnets, an approved policy and a live agent.
type discoverySite struct {
	admin            client
	rd, subnet       string
	blackedOutSubnet string
	policy           string
	author, approver *auth.Principal
	agent            []string // headers that sign in as the agent
}

func newDiscoverySite(t *testing.T, p *people, tx pgx.Tx) discoverySite {
	t.Helper()
	ctx := context.Background()
	author := p.user("disc-author@example.com", auth.SuperAdminRole)
	approver := p.user("disc-approver@example.com", auth.SuperAdminRole)
	c := p.as(author)
	org := id(c.must(201, "POST", "/organisations", gin.H{"name": "Disc Org", "code": "disc"}))
	rd := id(c.must(201, "POST", "/routing-domains", gin.H{"organisationId": org, "name": "Disc VRF", "kind": "vrf"}))
	subnet := id(c.must(201, "POST", "/subnets", gin.H{"routingDomainId": rd, "cidr": "10.77.1.0/24", "name": "Scan", "environment": "production"}))
	dark := id(c.must(201, "POST", "/subnets", gin.H{"routingDomainId": rd, "cidr": "10.77.2.0/24", "name": "Dark", "environment": "production"}))
	if _, err := tx.Exec(ctx, `UPDATE subnets SET discovery_enabled = true WHERE id = ANY($1::uuid[])`, []string{subnet, dark}); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO discovery_blackouts (name, target, starts_at, ends_at) VALUES ('Freeze', '10.77.2.0/24', now() - interval '1 hour', now() + interval '1 hour')`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO discovery_exclusions (target, routing_domain_id, reason) VALUES ('10.77.1.128/25', $1::uuid, 'Fragile')`, rd); err != nil {
		t.Fatal(err)
	}
	policy := id(c.must(201, "POST", "/discovery/policies", gin.H{"name": "Sweep", "methods": []string{"icmp"}, "targetAllowlist": []string{"10.77.0.0/16"},
		"maxConcurrency": 1, "maxPps": 10, "timeoutMs": 1000, "retries": 0}))
	p.as(approver).must(200, "POST", "/discovery/policies/"+policy+"/approve", nil)
	c = p.as(author)
	token := data(c.must(201, "POST", "/agents", gin.H{"name": "Disc Agent"}))["registrationToken"].(string)
	credential := data(c.must(200, "POST", "/agent-api/register", gin.H{"token": token, "version": "test"}))["credential"].(string)
	return discoverySite{c, rd, subnet, dark, policy, author, approver, []string{"Authorization", "Agent " + credential}}
}

func TestDiscoveryJobFromClaimToEvidence(t *testing.T) {
	p, tx := setupPeople(t)
	s := newDiscoverySite(t, p, tx)
	c := s.admin
	address := id(c.must(201, "POST", "/ip-addresses", gin.H{"routingDomainId": s.rd, "address": "10.77.1.5", "allocationState": "allocated"}))

	job := id(c.must(201, "POST", "/discovery/jobs", gin.H{"policyId": s.policy, "subnetIds": []string{s.subnet, s.blackedOutSubnet}, "priority": 1}))
	if n := count(t, tx, `SELECT count(*) FROM discovery_job_targets WHERE job_id = $1::uuid`, job); n != 1 {
		t.Fatalf("the excluded half and the blackout must leave one target: %d", n)
	}
	if n := count(t, tx, `SELECT count(*) FROM discovery_job_targets WHERE job_id = $1::uuid AND target = '10.77.1.0/25'`, job); n != 1 {
		t.Fatal("the target must be the part left after the exclusion")
	}
	blocked := id(c.must(201, "POST", "/discovery/jobs", gin.H{"policyId": s.policy, "subnetIds": []string{s.subnet}}))

	claim := data(c.must(200, "GET", "/agent-api/jobs", nil, s.agent...))
	if claim["jobId"] != job {
		t.Fatalf("claimed %v", claim["jobId"])
	}
	target := claim["targets"].([]any)[0].(map[string]any)["id"].(string)
	if again := data(c.must(200, "GET", "/agent-api/jobs", nil, s.agent...)); again["jobId"] != job {
		t.Fatalf("a restart must resume the running job, not take the overlapping %s: %v", blocked, again["jobId"])
	}

	early, late := time.Now().Add(-2*time.Hour).UTC(), time.Now().Add(-time.Hour).UTC()
	reply := data(c.must(200, "POST", "/agent-api/jobs/"+job+"/results", gin.H{
		"observations": []gin.H{
			{"method": "icmp", "address": "10.77.1.5", "alive": true, "observedAt": late, "mac": "aa:bb:cc:00:11:22", "hostname": "first"},
			{"method": "icmp", "address": "10.77.1.5", "alive": true, "observedAt": early},
			{"method": "icmp", "address": "10.77.1.6", "alive": false, "payload": gin.H{"rtt": 3}},
			{"method": "icmp", "address": "10.77.1.200", "alive": true}, // excluded
			{"method": "icmp", "address": "192.0.2.1", "alive": true},   // out of scope
			{"method": "telepathy", "address": "10.77.1.7", "alive": true},
			{"method": "icmp", "address": "not an address", "alive": true},
		},
		"targets":  []gin.H{{"id": target, "status": "running", "checkpoint": "a"}, {"id": target, "status": "done", "checkpoint": "b"}},
		"logs":     []gin.H{{"level": "shouting", "message": "one"}, {"level": "error", "message": "two"}},
		"progress": 40,
	}, s.agent...))
	if reply["instruction"] != "continue" {
		t.Fatalf("instruction: %v", reply)
	}
	if n := count(t, tx, `SELECT count(*) FROM discovery_observations WHERE job_id = $1::uuid`, job); n != 3 {
		t.Fatalf("observations kept: %d", n)
	}
	if n := count(t, tx, `SELECT count(*) FROM discovery_observations WHERE job_id = $1::uuid AND mac = 'aa:bb:cc:00:11:22' AND hostname = 'first'`, job); n != 1 {
		t.Fatal("mac and hostname lost")
	}
	if n := count(t, tx, `SELECT count(*) FROM discovery_observations WHERE job_id = $1::uuid AND payload->>'rtt' = '3' AND mac IS NULL AND hostname IS NULL`, job); n != 1 {
		t.Fatal("payload lost")
	}
	if n := count(t, tx, `SELECT count(*) FROM ip_addresses WHERE id = $1::uuid AND observation_state = 'observed_active' AND allocation_state = 'allocated'
		AND last_seen = $2 AND first_seen IS NOT NULL AND discovery_source_id IS NOT NULL`, address, late); n != 1 {
		t.Fatal("the address must show the latest sighting and keep its allocation state")
	}
	if n := count(t, tx, `SELECT count(*) FROM discovery_job_targets WHERE id = $1::uuid AND status = 'done' AND checkpoint = 'b'`, target); n != 1 {
		t.Fatal("the last report for a target must win")
	}
	if n := count(t, tx, `SELECT count(*) FROM discovery_job_logs WHERE job_id = $1::uuid AND level = 'info' AND message = 'one'`, job); n != 1 {
		t.Fatal("an unknown log level must become info")
	}
	if n := count(t, tx, `SELECT count(*) FROM discovery_job_logs WHERE job_id = $1::uuid AND level = 'warning' AND message LIKE 'Dropped 4 results%'`, job); n != 1 {
		t.Fatal("dropped results must be logged")
	}
	if n := count(t, tx, `SELECT count(*) FROM discovery_jobs WHERE id = $1::uuid AND progress = 40 AND status = 'running'`, job); n != 1 {
		t.Fatal("progress lost")
	}

	c.must(200, "POST", "/discovery/jobs/"+job+"/pause", nil)
	if got := data(c.must(200, "POST", "/agent-api/jobs/"+job+"/results", gin.H{}, s.agent...)); got["instruction"] != "pause" {
		t.Fatalf("instruction: %v", got)
	}
	c.must(200, "POST", "/discovery/jobs/"+job+"/resume", nil)
	exec(t, tx, `UPDATE discovery_job_targets SET status = 'pending' WHERE id = $1::uuid`, target)
	data(c.must(200, "GET", "/agent-api/jobs", nil, s.agent...))
	if got := data(c.must(200, "POST", "/agent-api/jobs/"+job+"/results", gin.H{"done": true}, s.agent...)); got["instruction"] != "continue" {
		t.Fatalf("instruction: %v", got)
	}
	if n := count(t, tx, `SELECT count(*) FROM discovery_jobs WHERE id = $1::uuid AND status = 'completed' AND progress = 100`, job); n != 1 {
		t.Fatal("the job must complete")
	}
}

func TestSchedulerAndEmergencyStop(t *testing.T) {
	p, tx := setupPeople(t)
	ctx := context.Background()
	s := newDiscoverySite(t, p, tx)
	var schedule string
	if err := tx.QueryRow(ctx, `INSERT INTO discovery_schedules (policy_id, cron, timezone, subnet_id, is_enabled) VALUES ($1::uuid, '* * * * *', 'UTC', $2::uuid, true) RETURNING id::text`,
		s.policy, s.subnet).Scan(&schedule); err != nil {
		t.Fatal(err)
	}
	svc := discovery.NewService(tx)
	now := time.Now()
	if res, err := svc.RunScheduler(ctx, now); err != nil || res.Started != 1 {
		t.Fatalf("%+v %v", res, err)
	}
	if res, err := svc.RunScheduler(ctx, now); err != nil || res.Started != 0 {
		t.Fatalf("a schedule fires once a minute: %+v %v", res, err)
	}
	if res, err := svc.RunScheduler(ctx, now.Add(time.Minute)); err != nil || res.Started != 0 {
		t.Fatalf("an unfinished job must not pile up: %+v %v", res, err)
	}
	if n := count(t, tx, `SELECT count(*) FROM discovery_jobs WHERE schedule_id = $1::uuid AND initiated_by IS NULL`, schedule); n != 1 {
		t.Fatalf("scheduled jobs: %d", n)
	}
	job := text(t, tx, `SELECT id::text FROM discovery_jobs WHERE schedule_id = $1::uuid`, schedule)

	stopped := data(s.admin.must(200, "POST", "/discovery/emergency-stop", gin.H{"active": true, "reason": "Incident"}))
	if stopped["emergencyStop"] != true {
		t.Fatalf("stop: %v", stopped)
	}
	if n := count(t, tx, `SELECT count(*) FROM discovery_jobs WHERE id = $1::uuid AND status = 'stopped'`, job); n != 1 {
		t.Fatal("the job must stop")
	}
	if n := count(t, tx, `SELECT count(*) FROM discovery_job_logs WHERE job_id = $1::uuid AND level = 'error' AND message = 'Stopped by the emergency stop: Incident'`, job); n != 1 {
		t.Fatal("the stop must be logged on the job")
	}
	if code, _ := s.admin.call("GET", "/agent-api/jobs", nil, s.agent...); code != 204 {
		t.Fatalf("no work during a stop: %d", code)
	}
	if code, _ := s.admin.call("POST", "/discovery/jobs", gin.H{"policyId": s.policy, "subnetIds": []string{s.subnet}}); code != 409 {
		t.Fatalf("no new jobs during a stop: %d", code)
	}
}
