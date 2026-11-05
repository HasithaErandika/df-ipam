package discovery

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/jackc/pgx/v5"
)

var (
	errStopped     = httpx.NewError(http.StatusConflict, "DISCOVERY_STOPPED", "The emergency stop is on. No discovery runs until it is lifted.")
	errNotApproved = httpx.NewError(http.StatusConflict, "POLICY_NOT_APPROVED", "This policy hasn't been approved, so it can't run.")
)

func targetError(msg string) error {
	return httpx.NewError(http.StatusUnprocessableEntity, "TARGET_NOT_ALLOWED", msg)
}

type Service struct{ conn db.Conn }

func NewService(conn db.Conn) *Service { return &Service{conn} }

type querier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// Stopped reports whether the emergency stop is on.
func Stopped(ctx context.Context, q querier) (bool, error) {
	var on bool
	err := q.QueryRow(ctx, `SELECT coalesce((SELECT value = 'true'::jsonb FROM system_settings WHERE key = 'discovery.emergency_stop'), false)`).Scan(&on)
	return on, err
}

// Policy is a discovery policy.
type Policy struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Description    string     `json:"description"`
	Methods        []string   `json:"methods"`
	Allowlist      []string   `json:"targetAllowlist"`
	MaxConcurrency int        `json:"maxConcurrency"`
	MaxPPS         int        `json:"maxPps"`
	TimeoutMS      int        `json:"timeoutMs"`
	Retries        int        `json:"retries"`
	Windows        []Window   `json:"windows"`
	IsSensitive    bool       `json:"isSensitive"`
	ApprovedBy     *string    `json:"approvedByEmail"`
	ApprovedAt     *time.Time `json:"approvedAt"`
	CreatedBy      *string    `json:"createdBy"`
	UpdatedBy      *string    `json:"updatedBy"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

const policyCols = `p.id::text, p.name, p.description, p.methods, p.target_allowlist::text[], p.max_concurrency, p.max_pps, p.timeout_ms,
	p.retries, p.windows, p.is_sensitive, u.email, p.approved_at, p.created_by::text, p.updated_by::text, p.updated_at`

func scanPolicy(r pgx.Row) (Policy, error) {
	var p Policy
	var windows []byte
	err := r.Scan(&p.ID, &p.Name, &p.Description, &p.Methods, &p.Allowlist, &p.MaxConcurrency, &p.MaxPPS, &p.TimeoutMS, &p.Retries,
		&windows, &p.IsSensitive, &p.ApprovedBy, &p.ApprovedAt, &p.CreatedBy, &p.UpdatedBy, &p.UpdatedAt)
	if err == nil {
		err = json.Unmarshal(windows, &p.Windows)
	}
	return p, err
}

func (s *Service) Policies(ctx context.Context) ([]Policy, error) {
	rows, err := s.conn.Query(ctx, "SELECT "+policyCols+" FROM discovery_policies p LEFT JOIN users u ON u.id = p.approved_by ORDER BY p.name")
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Policy, error) { return scanPolicy(r) })
	if out == nil {
		out = []Policy{}
	}
	return out, err
}

func (s *Service) policy(ctx context.Context, q querier, id string, lock bool) (Policy, error) {
	sql := "SELECT " + policyCols + " FROM discovery_policies p LEFT JOIN users u ON u.id = p.approved_by WHERE p.id = $1::uuid"
	if lock {
		sql += " FOR UPDATE OF p"
	}
	p, err := scanPolicy(q.QueryRow(ctx, sql, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return p, httpx.ErrNotFound
	}
	return p, err
}

// PolicyInput creates or replaces a policy. Saving always clears approval,
// because an approval covers the exact settings approved.
type PolicyInput struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Methods        []string `json:"methods"`
	Allowlist      []string `json:"targetAllowlist"`
	MaxConcurrency int      `json:"maxConcurrency"`
	MaxPPS         int      `json:"maxPps"`
	TimeoutMS      int      `json:"timeoutMs"`
	Retries        int      `json:"retries"`
	Windows        []Window `json:"windows"`
	IsSensitive    bool     `json:"isSensitive"`
}

func (in *PolicyInput) validate() error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return httpx.Validation("name", "Name the policy.")
	}
	if len(in.Methods) == 0 {
		return httpx.Validation("methods", "Choose at least one method.")
	}
	for _, m := range in.Methods {
		if !Methods[m] {
			return httpx.Validation("methods", "Unknown method "+m+".")
		}
	}
	if len(in.Allowlist) == 0 {
		return httpx.Validation("targetAllowlist", "List the networks this policy may touch.")
	}
	for i, c := range in.Allowlist {
		p, err := netip.ParsePrefix(strings.TrimSpace(c))
		if err != nil || p.Masked() != p {
			return httpx.Validation("targetAllowlist", "Enter networks as CIDRs, such as 10.20.0.0/16.")
		}
		in.Allowlist[i] = p.String()
	}
	if in.MaxConcurrency == 0 {
		in.MaxConcurrency = 16
	}
	if in.MaxPPS == 0 {
		in.MaxPPS = 50
	}
	if in.TimeoutMS == 0 {
		in.TimeoutMS = 1000
	}
	if in.MaxConcurrency < 1 || in.MaxConcurrency > 256 || in.MaxPPS < 1 || in.MaxPPS > 1000 || in.TimeoutMS < 100 || in.TimeoutMS > 30000 || in.Retries < 0 || in.Retries > 5 {
		return httpx.Validation("maxPps", "Limits: concurrency 1-256, packets per second 1-1000, timeout 100-30000 ms, retries 0-5.")
	}
	if in.Windows == nil {
		in.Windows = []Window{}
	}
	if err := ValidateWindows(in.Windows); err != nil {
		return httpx.Validation("windows", err.Error())
	}
	return nil
}

func (s *Service) SavePolicy(ctx context.Context, actor, id string, in PolicyInput) (Policy, error) {
	if err := in.validate(); err != nil {
		return Policy{}, err
	}
	windows, _ := json.Marshal(in.Windows)
	var out Policy
	err := db.InTx(ctx, s.conn, func(tx pgx.Tx) error {
		var old *Policy
		if id == "" {
			if err := tx.QueryRow(ctx, `INSERT INTO discovery_policies (name, created_by, updated_by) VALUES ($1, $2::uuid, $2::uuid) RETURNING id::text`,
				in.Name, actor).Scan(&id); err != nil {
				return err
			}
		} else {
			p, err := s.policy(ctx, tx, id, true)
			if err != nil {
				return err
			}
			old = &p
		}
		if _, err := tx.Exec(ctx, `UPDATE discovery_policies SET name = $2, description = $3, methods = $4, target_allowlist = $5::cidr[],
			max_concurrency = $6, max_pps = $7, timeout_ms = $8, retries = $9, windows = $10::jsonb, is_sensitive = $11,
			approved_by = NULL, approved_at = NULL, updated_by = $12::uuid WHERE id = $1::uuid`,
			id, in.Name, in.Description, in.Methods, in.Allowlist, in.MaxConcurrency, in.MaxPPS, in.TimeoutMS, in.Retries, windows, in.IsSensitive, actor); err != nil {
			return err
		}
		var err error
		if out, err = s.policy(ctx, tx, id, false); err != nil {
			return err
		}
		action := "discovery_policy.create"
		if old != nil {
			action = "discovery_policy.update"
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: action, ObjectType: "discovery_policy", ObjectID: id, Old: old, New: out})
	})
	if db.ErrorCode(err) == db.CodeUnique {
		err = httpx.NewError(http.StatusConflict, "ALREADY_EXISTS", "A policy with this name already exists.")
	}
	return out, err
}

// ApprovePolicy records approval by someone who didn't write the settings.
func (s *Service) ApprovePolicy(ctx context.Context, actor, id string) (Policy, error) {
	var out Policy
	err := db.InTx(ctx, s.conn, func(tx pgx.Tx) error {
		p, err := s.policy(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if (p.CreatedBy != nil && *p.CreatedBy == actor) || (p.UpdatedBy != nil && *p.UpdatedBy == actor) {
			return httpx.NewError(http.StatusForbidden, "SELF_APPROVAL_DENIED", "Someone other than the person who wrote this policy must approve it.")
		}
		if _, err := tx.Exec(ctx, `UPDATE discovery_policies SET approved_by = $2::uuid, approved_at = now() WHERE id = $1::uuid`, id, actor); err != nil {
			return err
		}
		if out, err = s.policy(ctx, tx, id, false); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "discovery_policy.approve", ObjectType: "discovery_policy", ObjectID: id, ApprovalRef: id, New: out})
	})
	return out, err
}

// JobRequest asks for discovery of whole subnets under a policy.
type JobRequest struct {
	PolicyID  string   `json:"policyId"`
	SubnetIDs []string `json:"subnetIds"`
	AgentID   *string  `json:"agentId"`
	Methods   []string `json:"methods"`
	Priority  int      `json:"priority"`
}

// Snapshot is a job's frozen configuration. The database refuses changes to it.
type Snapshot struct {
	PolicyID       string    `json:"policyId"`
	PolicyName     string    `json:"policyName"`
	Methods        []string  `json:"methods"`
	MaxConcurrency int       `json:"maxConcurrency"`
	MaxPPS         int       `json:"maxPps"`
	TimeoutMS      int       `json:"timeoutMs"`
	Retries        int       `json:"retries"`
	Windows        []Window  `json:"windows"`
	Targets        []Target  `json:"targets"`
	Skipped        []string  `json:"skipped"`
	FrozenAt       time.Time `json:"frozenAt"`
}

type subnetScope struct {
	ID, RoutingDomainID, CIDR, Name, Sensitivity string
	Enabled, ServiceSensitive                    bool
}

func activeExclusions(ctx context.Context, q querier) ([]Exclusion, error) {
	rows, err := q.Query(ctx, `SELECT coalesce(routing_domain_id::text, ''), target::text FROM discovery_exclusions WHERE expires_at IS NULL OR expires_at > now()`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Exclusion
	for rows.Next() {
		var rd, cidr string
		if err := rows.Scan(&rd, &cidr); err != nil {
			return nil, err
		}
		if p, err := netip.ParsePrefix(cidr); err == nil {
			out = append(out, Exclusion{rd, p})
		}
	}
	return out, rows.Err()
}

func blackedOut(ctx context.Context, q querier, cidr string) (bool, error) {
	var hit bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM discovery_blackouts WHERE now() >= starts_at AND now() < ends_at
		AND (target IS NULL OR target && $1::cidr))`, cidr).Scan(&hit)
	return hit, err
}

// CreateJob freezes a job's scope from approved policy and subnets. actor is
// empty for scheduled jobs.
func (s *Service) CreateJob(ctx context.Context, tx pgx.Tx, actor string, req JobRequest, scheduleID *string) (string, error) {
	if on, err := Stopped(ctx, tx); err != nil {
		return "", err
	} else if on {
		return "", errStopped
	}
	if !httpx.IsUUID(req.PolicyID) {
		return "", httpx.Validation("policyId", "Choose a policy.")
	}
	if len(req.SubnetIDs) == 0 || len(req.SubnetIDs) > 100 {
		return "", httpx.Validation("subnetIds", "Choose between 1 and 100 subnets.")
	}
	p, err := s.policy(ctx, tx, req.PolicyID, false)
	if errors.Is(err, httpx.ErrNotFound) {
		return "", httpx.Validation("policyId", "That policy doesn't exist.")
	}
	if err != nil {
		return "", err
	}
	if p.ApprovedAt == nil {
		return "", errNotApproved
	}
	methods := p.Methods
	if len(req.Methods) > 0 {
		var chosen []string
		for _, m := range req.Methods {
			for _, pm := range p.Methods {
				if m == pm {
					chosen = append(chosen, m)
				}
			}
		}
		methods = chosen
	}
	if len(methods) == 0 {
		return "", httpx.Validation("methods", "Choose methods the policy allows.")
	}
	var allow []netip.Prefix
	for _, a := range p.Allowlist {
		if pr, err := netip.ParsePrefix(a); err == nil {
			allow = append(allow, pr)
		}
	}
	exclusions, err := activeExclusions(ctx, tx)
	if err != nil {
		return "", err
	}
	snap := Snapshot{PolicyID: p.ID, PolicyName: p.Name, MaxConcurrency: p.MaxConcurrency, MaxPPS: p.MaxPPS, TimeoutMS: p.TimeoutMS,
		Retries: p.Retries, Windows: p.Windows, FrozenAt: time.Now().UTC(), Skipped: []string{}}
	methodSet := map[string]bool{}
	var firstSubnet *string
	for _, id := range req.SubnetIDs {
		if !httpx.IsUUID(id) {
			return "", httpx.Validation("subnetIds", "Every subnet ID must be valid.")
		}
		var sc subnetScope
		err := tx.QueryRow(ctx, `SELECT s.id::text, s.routing_domain_id::text, s.cidr::text, s.name, s.sensitivity, s.discovery_enabled,
			coalesce(bs.is_sensitive, false) FROM subnets s LEFT JOIN business_services bs ON bs.id = s.business_service_id
			WHERE s.id = $1::uuid AND s.archived_at IS NULL`, id).
			Scan(&sc.ID, &sc.RoutingDomainID, &sc.CIDR, &sc.Name, &sc.Sensitivity, &sc.Enabled, &sc.ServiceSensitive)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", httpx.Validation("subnetIds", "A chosen subnet doesn't exist or is archived.")
		}
		if err != nil {
			return "", err
		}
		if firstSubnet == nil {
			firstSubnet = &sc.ID
		}
		pr := netip.MustParsePrefix(sc.CIDR)
		if !sc.Enabled {
			return "", targetError(fmt.Sprintf("Discovery is turned off for %s (%s).", sc.Name, sc.CIDR))
		}
		if !Allowed(pr, allow) {
			return "", targetError(fmt.Sprintf("%s (%s) is outside the policy's approved networks.", sc.Name, sc.CIDR))
		}
		if hit, err := blackedOut(ctx, tx, sc.CIDR); err != nil {
			return "", err
		} else if hit {
			snap.Skipped = append(snap.Skipped, sc.CIDR+": in a maintenance blackout")
			continue
		}
		sensitive := sc.Sensitivity != "standard" || sc.ServiceSensitive
		allowed := FilterMethods(methods, sensitive, p.IsSensitive)
		if len(allowed) == 0 {
			snap.Skipped = append(snap.Skipped, sc.CIDR+": sensitive network, passive methods only")
			continue
		}
		for _, m := range allowed {
			methodSet[m] = true
		}
		parts := Subtract(pr, sc.RoutingDomainID, exclusions)
		if len(parts) == 0 {
			snap.Skipped = append(snap.Skipped, sc.CIDR+": fully excluded")
			continue
		}
		for _, part := range parts {
			if TooLarge(part) {
				if part.Addr().Is6() {
					// IPv6 is never swept: only addresses already recorded are probed.
					rows, err := tx.Query(ctx, `SELECT host(address) FROM ip_addresses WHERE subnet_id = $1::uuid AND address <<= $2::cidr
						ORDER BY address LIMIT $3`, sc.ID, part.String(), MaxKnownTargets)
					if err != nil {
						return "", err
					}
					hosts, err := pgx.CollectRows(rows, pgx.RowTo[string])
					if err != nil {
						return "", err
					}
					for _, h := range hosts {
						snap.Targets = append(snap.Targets, Target{sc.RoutingDomainID, h + "/128"})
					}
					if len(hosts) == 0 {
						snap.Skipped = append(snap.Skipped, part.String()+": IPv6 subnet with no recorded addresses to probe")
					}
					continue
				}
				return "", httpx.NewError(http.StatusUnprocessableEntity, "TARGET_TOO_LARGE",
					fmt.Sprintf("%s is larger than /%d. Split it into smaller subnets to discover it.", part, MaxIPv4Bits))
			}
			snap.Targets = append(snap.Targets, Target{sc.RoutingDomainID, part.String()})
		}
	}
	if len(snap.Targets) == 0 {
		return "", targetError("Nothing is left to discover after exclusions, blackouts and sensitivity rules: " + strings.Join(snap.Skipped, "; "))
	}
	if len(snap.Targets) > MaxTargets {
		return "", targetError("This job would have too many targets. Choose fewer subnets.")
	}
	for m := range methodSet {
		snap.Methods = append(snap.Methods, m)
	}
	raw, _ := json.Marshal(snap)
	priority := req.Priority
	if priority < 1 || priority > 9 {
		priority = 5
	}
	var initiator *string
	if actor != "" {
		initiator = &actor
	}
	var id string
	if err := tx.QueryRow(ctx, `INSERT INTO discovery_jobs (policy_id, agent_id, status, priority, config_snapshot, initiated_by, schedule_id, subnet_id)
		VALUES ($1::uuid, $2::uuid, 'queued', $3, $4::jsonb, $5::uuid, $6::uuid, $7::uuid) RETURNING id::text`,
		p.ID, req.AgentID, priority, raw, initiator, scheduleID, firstSubnet).Scan(&id); err != nil {
		return "", err
	}
	for _, t := range snap.Targets {
		if _, err := tx.Exec(ctx, `INSERT INTO discovery_job_targets (job_id, routing_domain_id, target) VALUES ($1::uuid, $2::uuid, $3::cidr)`,
			id, t.RoutingDomainID, t.CIDR); err != nil {
			return "", err
		}
	}
	msg := fmt.Sprintf("Job created with %d targets and methods %s.", len(snap.Targets), strings.Join(snap.Methods, ", "))
	if len(snap.Skipped) > 0 {
		msg += " Skipped: " + strings.Join(snap.Skipped, "; ")
	}
	if err := Log(ctx, tx, id, "info", msg); err != nil {
		return "", err
	}
	actorType := "user"
	if actor == "" {
		actorType = "system"
	}
	return id, audit.Write(ctx, tx, audit.Entry{ActorID: actor, ActorType: actorType, Action: "discovery_job.create", ObjectType: "discovery_job", ObjectID: id, New: snap})
}

// Log appends a job log line.
func Log(ctx context.Context, tx pgx.Tx, jobID, level, message string) error {
	if len(message) > 2000 {
		message = message[:2000]
	}
	_, err := tx.Exec(ctx, `INSERT INTO discovery_job_logs (job_id, level, message) VALUES ($1::uuid, $2, $3)`, jobID, level, message)
	return err
}

// Transitions people may make to a job.
var transitions = map[string]struct {
	from []string
	to   string
}{
	"pause":  {[]string{"queued", "running"}, "paused"},
	"resume": {[]string{"paused"}, "queued"},
	"cancel": {[]string{"queued", "running", "paused"}, "cancelled"},
}

func (s *Service) Control(ctx context.Context, actor, id, action string) error {
	t, ok := transitions[action]
	if !ok {
		return httpx.ErrNotFound
	}
	return db.InTx(ctx, s.conn, func(tx pgx.Tx) error {
		var status string
		if err := tx.QueryRow(ctx, `SELECT status FROM discovery_jobs WHERE id = $1::uuid FOR UPDATE`, id).Scan(&status); errors.Is(err, pgx.ErrNoRows) {
			return httpx.ErrNotFound
		} else if err != nil {
			return err
		}
		allowed := false
		for _, f := range t.from {
			allowed = allowed || f == status
		}
		if !allowed {
			return httpx.NewError(http.StatusConflict, "INVALID_STATE", "A "+status+" job can't be "+action+"d.")
		}
		if action == "resume" {
			if on, err := Stopped(ctx, tx); err != nil {
				return err
			} else if on {
				return errStopped
			}
		}
		finish := ""
		if t.to == "cancelled" {
			finish = ", finished_at = now()"
		}
		if _, err := tx.Exec(ctx, `UPDATE discovery_jobs SET status = $2`+finish+` WHERE id = $1::uuid`, id, t.to); err != nil {
			return err
		}
		if err := Log(ctx, tx, id, "info", "Job "+t.to+" by a person."); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "discovery_job." + action, ObjectType: "discovery_job", ObjectID: id,
			Old: map[string]string{"status": status}, New: map[string]string{"status": t.to}})
	})
}

// EmergencyStop turns the stop on or off. Turning it on stops every queued,
// running and paused job at once; agents halt at their next contact.
func (s *Service) EmergencyStop(ctx context.Context, actor string, on bool, reason string) (int, error) {
	if on && strings.TrimSpace(reason) == "" {
		return 0, httpx.Validation("reason", "Say why discovery is being stopped.")
	}
	n := 0
	err := db.InTx(ctx, s.conn, func(tx pgx.Tx) error {
		value := "false"
		if on {
			value = "true"
		}
		if _, err := tx.Exec(ctx, `INSERT INTO system_settings (key, value, updated_by, updated_at) VALUES ('discovery.emergency_stop', $1::jsonb, $2::uuid, now())
			ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by, updated_at = now()`, value, actor); err != nil {
			return err
		}
		if on {
			rows, err := tx.Query(ctx, `UPDATE discovery_jobs SET status = 'stopped', finished_at = now(), error = 'Emergency stop: ' || $1
				WHERE status IN ('queued','running','paused') RETURNING id::text`, reason)
			if err != nil {
				return err
			}
			ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
			if err != nil {
				return err
			}
			n = len(ids)
			for _, id := range ids {
				if err := Log(ctx, tx, id, "error", "Stopped by the emergency stop: "+reason); err != nil {
					return err
				}
			}
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "discovery.emergency_stop", ObjectType: "system_setting",
			ObjectID: "discovery.emergency_stop", New: map[string]any{"active": on, "reason": reason, "jobsStopped": n}})
	})
	return n, err
}

// NewSecret returns a random secret and its stored hash.
func NewSecret() (string, string) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	secret := base64.RawURLEncoding.EncodeToString(b)
	return secret, Hash(secret)
}

// Hash is how agent secrets are stored: never in plain text.
func Hash(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// RegistrationTTL is how long a registration token stays usable.
const RegistrationTTL = 24 * time.Hour

// IssueToken creates a single-use registration token for an agent.
func IssueToken(ctx context.Context, tx pgx.Tx, actor, agentID string) (string, time.Time, error) {
	token, hash := NewSecret()
	expires := time.Now().Add(RegistrationTTL).UTC()
	if _, err := tx.Exec(ctx, `UPDATE agent_registration_tokens SET expires_at = now() WHERE agent_id = $1::uuid AND used_at IS NULL AND expires_at > now()`, agentID); err != nil {
		return "", expires, err
	}
	_, err := tx.Exec(ctx, `INSERT INTO agent_registration_tokens (agent_id, token_hash, created_by, expires_at) VALUES ($1::uuid, $2, $3::uuid, $4)`,
		agentID, hash, actor, expires)
	return token, expires, err
}
