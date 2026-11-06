package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/platform/httpx"

	"github.com/jackc/pgx/v5"
)

// CreateJob freezes a job's scope from approved policy and subnets. actor is
// empty for scheduled jobs.
func (s *Service) CreateJob(ctx context.Context, tx pgx.Tx, actor string, req JobRequest, scheduleID *string) (string, error) {
	if err := checkJobRequest(ctx, tx, req); err != nil {
		return "", err
	}
	p, err := s.runnablePolicy(ctx, tx, req.PolicyID)
	if err != nil {
		return "", err
	}
	methods := chooseMethods(p.Methods, req.Methods)
	if len(methods) == 0 {
		return "", httpx.Validation("methods", "Choose methods the policy allows.")
	}
	scopes, err := loadSubnetScopes(ctx, tx, req.SubnetIDs)
	if err != nil {
		return "", err
	}
	snap, err := freezeScope(ctx, tx, p, methods, scopes)
	if err != nil {
		return "", err
	}
	id, err := insertJob(ctx, tx, jobRow{p.ID, req.AgentID, req.Priority, actor, scheduleID, scopes[0].ID}, snap)
	if err != nil {
		return "", err
	}
	return id, recordJobCreated(ctx, tx, id, actor, snap)
}

func checkJobRequest(ctx context.Context, tx pgx.Tx, req JobRequest) error {
	if on, err := Stopped(ctx, tx); err != nil {
		return err
	} else if on {
		return errStopped
	}
	if !httpx.IsUUID(req.PolicyID) {
		return httpx.Validation("policyId", "Choose a policy.")
	}
	if len(req.SubnetIDs) == 0 || len(req.SubnetIDs) > 100 {
		return httpx.Validation("subnetIds", "Choose between 1 and 100 subnets.")
	}
	return nil
}

func (s *Service) runnablePolicy(ctx context.Context, tx pgx.Tx, id string) (Policy, error) {
	p, err := s.policy(ctx, tx, id, false)
	if errors.Is(err, httpx.ErrNotFound) {
		return p, httpx.Validation("policyId", "That policy doesn't exist.")
	}
	if err != nil {
		return p, err
	}
	if p.ApprovedAt == nil {
		return p, errNotApproved
	}
	return p, nil
}

// chooseMethods keeps the requested methods the policy allows, or all of the
// policy's methods when none were requested.
func chooseMethods(allowed, requested []string) []string {
	if len(requested) == 0 {
		return allowed
	}
	var chosen []string
	for _, m := range requested {
		if slices.Contains(allowed, m) {
			chosen = append(chosen, m)
		}
	}
	return chosen
}

type jobRow struct {
	policyID    string
	agentID     *string
	priority    int
	actor       string
	scheduleID  *string
	firstSubnet string
}

func insertJob(ctx context.Context, tx pgx.Tx, j jobRow, snap Snapshot) (string, error) {
	raw, _ := json.Marshal(snap)
	priority := j.priority
	if priority < 1 || priority > 9 {
		priority = 5
	}
	var initiator *string
	if j.actor != "" {
		initiator = &j.actor
	}
	var id string
	if err := tx.QueryRow(ctx, `INSERT INTO discovery_jobs (policy_id, agent_id, status, priority, config_snapshot, initiated_by, schedule_id, subnet_id)
		VALUES ($1::uuid, $2::uuid, 'queued', $3, $4::jsonb, $5::uuid, $6::uuid, $7::uuid) RETURNING id::text`,
		j.policyID, j.agentID, priority, raw, initiator, j.scheduleID, j.firstSubnet).Scan(&id); err != nil {
		return "", err
	}
	domains, cidrs := make([]string, len(snap.Targets)), make([]string, len(snap.Targets))
	for i, t := range snap.Targets {
		domains[i], cidrs[i] = t.RoutingDomainID, t.CIDR
	}
	_, err := tx.Exec(ctx, `INSERT INTO discovery_job_targets (job_id, routing_domain_id, target)
		SELECT $1::uuid, rd, cidr::cidr FROM unnest($2::uuid[], $3::text[]) AS t(rd, cidr)`, id, domains, cidrs)
	return id, err
}

func recordJobCreated(ctx context.Context, tx pgx.Tx, id, actor string, snap Snapshot) error {
	msg := fmt.Sprintf("Job created with %d targets and methods %s.", len(snap.Targets), strings.Join(snap.Methods, ", "))
	if len(snap.Skipped) > 0 {
		msg += " Skipped: " + strings.Join(snap.Skipped, "; ")
	}
	if err := Log(ctx, tx, id, "info", msg); err != nil {
		return err
	}
	actorType := "user"
	if actor == "" {
		actorType = "system"
	}
	return audit.Write(ctx, tx, audit.Entry{ActorID: actor, ActorType: actorType, Action: "discovery_job.create", ObjectType: "discovery_job", ObjectID: id, New: snap})
}
