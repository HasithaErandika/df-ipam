package agent

import (
	"context"
	"log/slog"
	"net/netip"
	"strconv"
	"time"
)

const postBatch = 4000 // stays under the server's limit of observations per call

// work claims one job and runs it target by target, posting after each, so a
// pause, stop or emergency stop takes effect between targets.
func (a *Agent) work(ctx context.Context) error {
	job, err := a.client.Claim(ctx)
	if err != nil || job == nil {
		return err
	}
	a.jobs.Add(1)
	cfg := limited(job.Config, a.cfg.MaxPPS)
	slog.Info("job claimed", "job", job.ID, "policy", cfg.PolicyName, "targets", len(job.Targets), "methods", cfg.Methods, "pps", cfg.MaxPPS)
	jctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go a.watchStop(jctx, cancel)
	closed := map[string]bool{}
	for i, target := range job.Targets {
		last := i == len(job.Targets)-1
		if closed[target.ID] {
			if last {
				return a.post(ctx, job.ID, Results{Progress: 100, Done: true}, nil, closed)
			}
			continue
		}
		obs, logs, status := a.scan(jctx, cfg, target)
		if jctx.Err() != nil {
			_, _, _ = a.client.Post(ctx, job.ID, Results{Logs: []LogLine{{"warning", "Agent halted mid-target: the emergency stop is on, or it lost contact with the server."}}})
			return errHalt
		}
		res := Results{Targets: []TargetUpdate{{ID: target.ID, Status: status}}, Logs: logs, Progress: (i + 1) * 100 / len(job.Targets), Done: last}
		if err := a.post(ctx, job.ID, res, obs, closed); err != nil {
			return err
		}
	}
	slog.Info("job finished", "job", job.ID)
	return nil
}

// watchStop cancels work in flight when the emergency stop arrives, or when the
// server can no longer send it.
func (a *Agent) watchStop(ctx context.Context, cancel context.CancelFunc) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if a.mustHalt() {
				cancel()
				return
			}
		}
	}
}

// post sends a target's results in batches. Only the last one carries the target
// update and the end of the job.
func (a *Agent) post(ctx context.Context, jobID string, res Results, obs []Observation, closed map[string]bool) error {
	for start := 0; ; start += postBatch {
		end := min(start+postBatch, len(obs))
		batch := res
		batch.Observations = obs[start:end]
		if end < len(obs) {
			batch.Done, batch.Targets = false, nil
		}
		instruction, skip, err := a.client.Post(ctx, jobID, batch)
		if err != nil {
			return err
		}
		for _, id := range skip {
			closed[id] = true
		}
		if instruction != "continue" {
			slog.Info("server halted the job", "job", jobID, "instruction", instruction)
			return errHalt
		}
		if end >= len(obs) {
			return nil
		}
	}
}

// targetScan gathers one target's results as each method runs.
type targetScan struct {
	cfg   JobConfig
	cidr  string
	hosts []netip.Addr
	alive []netip.Addr
	obs   []Observation
	logs  []LogLine
}

func (s *targetScan) log(level, message string) { s.logs = append(s.logs, LogLine{level, message}) }

func (a *Agent) scan(ctx context.Context, cfg JobConfig, t JobTarget) ([]Observation, []LogLine, string) {
	hosts, err := Hosts(t.CIDR)
	if err != nil {
		return nil, []LogLine{{"error", err.Error()}}, "failed"
	}
	s := &targetScan{cfg: cfg, cidr: t.CIDR, hosts: hosts}
	if has(cfg.Methods, "icmp") {
		s.ping(ctx)
	}
	if has(cfg.Methods, "nmap") {
		s.nmap(ctx)
	}
	if has(cfg.Methods, "arp") || has(cfg.Methods, "ndp") {
		s.neighbours(ctx)
	}
	if has(cfg.Methods, "dns") {
		s.reverseDNS(ctx)
	}
	if has(cfg.Methods, "snmp") {
		s.log("warning", "SNMP isn't collected yet: it needs SNMPv3 credentials, which arrive with integration credential storage.")
	}
	s.log("info", t.CIDR+": "+strconv.Itoa(len(s.obs))+" results.")
	return s.obs, s.logs, "done"
}

func observedNow() time.Time { return time.Now().UTC() }

func (s *targetScan) ping(ctx context.Context) {
	alive, err := Ping(ctx, s.hosts, s.cfg)
	if err != nil {
		s.log("error", "ICMP on "+s.cidr+": "+err.Error())
	}
	s.alive = alive
	for _, h := range alive {
		s.obs = append(s.obs, Observation{Method: "icmp", Address: h.String(), Alive: true, ObservedAt: observedNow()})
	}
}

func (s *targetScan) nmap(ctx context.Context) {
	found, err := Nmap(ctx, s.cidr, s.cfg)
	if err != nil {
		s.log("warning", "Nmap on "+s.cidr+": "+err.Error())
	}
	for _, h := range found {
		s.obs = append(s.obs, Observation{Method: "nmap", Address: h.Address, MAC: h.MAC, Hostname: h.Hostname, Alive: true,
			Payload: map[string]any{"vendor": h.Vendor}, ObservedAt: observedNow()})
		if a, err := netip.ParseAddr(h.Address); err == nil {
			s.alive = append(s.alive, a)
		}
	}
}

func (s *targetScan) neighbours(ctx context.Context) {
	p, _ := netip.ParsePrefix(s.cidr)
	for addr, mac := range Neighbours(ctx) {
		method := "arp"
		if addr.Is6() {
			method = "ndp"
		}
		if p.Contains(addr) && has(s.cfg.Methods, method) {
			s.obs = append(s.obs, Observation{Method: method, Address: addr.String(), MAC: mac, Alive: true, ObservedAt: observedNow()})
		}
	}
}

func (s *targetScan) reverseDNS(ctx context.Context) {
	lookup := s.alive
	if len(lookup) == 0 {
		lookup = s.hosts // passive-only jobs ask DNS about every address in the target
	}
	for addr, name := range ReverseDNS(ctx, lookup, s.cfg.MaxPPS) {
		s.obs = append(s.obs, Observation{Method: "dns", Address: addr.String(), Hostname: name, ObservedAt: observedNow(),
			Payload: map[string]any{"ptr": name}})
	}
}
