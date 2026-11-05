package server

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

func enabledSubnet(t *testing.T, tx pgx.Tx, s discoverySite, cidr, name string) string {
	t.Helper()
	id := id(s.admin.must(201, "POST", "/subnets", gin.H{"routingDomainId": s.rd, "cidr": cidr, "name": name, "environment": "production"}))
	if _, err := tx.Exec(context.Background(), `UPDATE subnets SET discovery_enabled = true WHERE id = $1::uuid`, id); err != nil {
		t.Fatal(err)
	}
	return id
}

func exec(t *testing.T, tx pgx.Tx, sql string, args ...any) {
	t.Helper()
	if _, err := tx.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

// approvedPolicy makes a policy over all of 10/8 with these windows.
func approvedPolicy(p *people, s discoverySite, name string, windows []gin.H) string {
	c := p.as(s.author)
	body := gin.H{"name": name, "methods": []string{"icmp"}, "targetAllowlist": []string{"10.0.0.0/8", "224.0.0.0/4"}, "maxConcurrency": 1, "maxPps": 10, "timeoutMs": 1000, "windows": windows}
	policy := id(c.must(201, "POST", "/discovery/policies", body))
	p.as(s.approver).must(200, "POST", "/discovery/policies/"+policy+"/approve", nil)
	p.as(s.author)
	return policy
}

func TestAgentNeverGetsMoreThanTheCeiling(t *testing.T) {
	p, tx := setupPeople(t)
	s := newDiscoverySite(t, p, tx)
	exec(t, tx, `INSERT INTO system_settings (key, value) VALUES ('discovery.max_total_pps', '4') ON CONFLICT (key) DO UPDATE SET value = '4'`)
	s.admin.must(201, "POST", "/discovery/jobs", gin.H{"policyId": s.policy, "subnetIds": []string{s.subnet}})
	config := data(s.admin.must(200, "GET", "/agent-api/jobs", nil, s.agent...))["config"].(map[string]any)
	if config["maxPps"] != float64(4) {
		t.Fatalf("policy 10 under a ceiling of 4 was granted %v", config["maxPps"])
	}
}

func TestBlackoutStopsTargetsOfARunningJob(t *testing.T) {
	p, tx := setupPeople(t)
	s := newDiscoverySite(t, p, tx)
	other := enabledSubnet(t, tx, s, "10.77.3.0/24", "Later")
	job := id(s.admin.must(201, "POST", "/discovery/jobs", gin.H{"policyId": s.policy, "subnetIds": []string{s.subnet, other}}))
	s.admin.must(200, "GET", "/agent-api/jobs", nil, s.agent...)
	exec(t, tx, `INSERT INTO discovery_blackouts (name, target, starts_at, ends_at) VALUES ('Change', '10.77.3.0/24', now() - interval '1 minute', now() + interval '1 hour')`)
	reply := data(s.admin.must(200, "POST", "/agent-api/jobs/"+job+"/results", gin.H{}, s.agent...))
	if skip := reply["skipTargets"].([]any); len(skip) != 1 || reply["instruction"] != "continue" {
		t.Fatalf("reply: %v", reply)
	}
	if n := count(t, tx, `SELECT count(*) FROM discovery_job_targets WHERE job_id = $1::uuid AND target = '10.77.3.0/24' AND status = 'skipped'`, job); n != 1 {
		t.Fatal("the blacked out target must be closed")
	}
	if n := count(t, tx, `SELECT count(*) FROM discovery_job_targets WHERE job_id = $1::uuid AND status = 'pending'`, job); n != 1 {
		t.Fatal("other targets carry on")
	}
}

func TestExclusionAddedBeforeClaimIsNeverProbed(t *testing.T) {
	p, tx := setupPeople(t)
	s := newDiscoverySite(t, p, tx)
	job := id(s.admin.must(201, "POST", "/discovery/jobs", gin.H{"policyId": s.policy, "subnetIds": []string{s.subnet}}))
	exec(t, tx, `INSERT INTO discovery_exclusions (target, routing_domain_id, reason) VALUES ('10.77.1.0/26', $1::uuid, 'Late')`, s.rd)
	if code, out := s.admin.call("GET", "/agent-api/jobs", nil, s.agent...); code != 204 {
		t.Fatalf("no target may reach the agent: %d %v", code, out)
	}
	if n := count(t, tx, `SELECT count(*) FROM discovery_jobs WHERE id = $1::uuid AND status = 'completed'`, job); n != 1 {
		t.Fatal("a job with nothing left must finish, not hang")
	}
}

func TestClosedWindowPausesARunningJob(t *testing.T) {
	p, tx := setupPeople(t)
	s := newDiscoverySite(t, p, tx)
	colombo := time.Now().UTC().Add(5*time.Hour + 30*time.Minute)
	otherDay := (int(colombo.Weekday())+3)%7 + 1 // never today or yesterday
	policy := approvedPolicy(p, s, "Night only", []gin.H{{"days": []int{otherDay}, "start": "01:00", "end": "02:00"}})
	job := id(s.admin.must(201, "POST", "/discovery/jobs", gin.H{"policyId": policy, "subnetIds": []string{s.subnet}}))
	if code, _ := s.admin.call("GET", "/agent-api/jobs", nil, s.agent...); code != 204 {
		t.Fatalf("outside its window a job must not start: %d", code)
	}
	agent := strings.SplitN(strings.TrimPrefix(s.agent[1], "Agent "), ".", 2)[0]
	exec(t, tx, `UPDATE discovery_jobs SET status = 'running', agent_id = $2::uuid WHERE id = $1::uuid`, job, agent)
	if got := data(s.admin.must(200, "POST", "/agent-api/jobs/"+job+"/results", gin.H{}, s.agent...)); got["instruction"] != "pause" {
		t.Fatalf("reply: %v", got)
	}
	if n := count(t, tx, `SELECT count(*) FROM discovery_jobs WHERE id = $1::uuid AND status = 'queued'`, job); n != 1 {
		t.Fatal("the job must wait for its next window")
	}
}

func TestOversizedAndFanOutJobsAreRefused(t *testing.T) {
	p, tx := setupPeople(t)
	s := newDiscoverySite(t, p, tx)
	policy := approvedPolicy(p, s, "Wide", nil)
	var ids []string
	for i := range 17 {
		ids = append(ids, enabledSubnet(t, tx, s, fmt.Sprintf("10.%d.0.0/20", 100+i), fmt.Sprintf("Big %d", i)))
	}
	code, out := s.admin.call("POST", "/discovery/jobs", gin.H{"policyId": policy, "subnetIds": ids})
	if code != 422 || !strings.Contains(fmt.Sprint(out), "the limit is 65536") {
		t.Fatalf("job size: %d %v", code, out)
	}
	multicast := enabledSubnet(t, tx, s, "224.1.1.0/24", "Multicast")
	if code, out := s.admin.call("POST", "/discovery/jobs", gin.H{"policyId": policy, "subnetIds": []string{multicast}}); code != 422 || errCode(out) != "TARGET_NOT_ALLOWED" {
		t.Fatalf("multicast: %d %v", code, out)
	}
}

// The app role is what a SQL injection or stolen password would have.
func TestAppRoleCannotDamageHistoryOrPeople(t *testing.T) {
	_, tx := setupPeople(t)
	ctx := context.Background()
	for _, sql := range []string{`DELETE FROM users`, `TRUNCATE users CASCADE`, `UPDATE schema_migrations SET version = 1`, `SELECT * FROM schema_migrations`,
		`SELECT * FROM sessions`, `DELETE FROM audit_logs`, `UPDATE audit_logs SET action = 'x'`, `DELETE FROM discovery_observations`} {
		sp, err := tx.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		_, err = sp.Exec(ctx, sql)
		_ = sp.Rollback(ctx)
		if err == nil || !strings.Contains(err.Error(), "42501") {
			t.Errorf("%s must be refused: %v", sql, err)
		}
	}
}
