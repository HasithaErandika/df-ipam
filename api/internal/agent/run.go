package agent

import (
	"context"
	"errors"
	"log/slog"
	"net/netip"
	"runtime"
	"sync/atomic"
	"time"
)

// errHalt means the server said to pause or stop, or the emergency stop is on.
var errHalt = errors.New("halted by the server")

// Agent ties the loop together.
type Agent struct {
	client  *Client
	cfg     Config
	stopped atomic.Bool
	jobs    atomic.Int64
}

func New(cfg Config) *Agent { return &Agent{client: NewClient(cfg), cfg: cfg} }

// Run registers if needed, then heartbeats and works until ctx ends.
func (a *Agent) Run(ctx context.Context) error {
	if err := a.client.Authenticate(ctx); err != nil {
		return err
	}
	slog.Info("agent authenticated", "api", a.cfg.APIURL, "version", Version)
	go a.heartbeats(ctx)
	tick := time.NewTicker(a.cfg.Poll)
	defer tick.Stop()
	for {
		if !a.stopped.Load() {
			if err := a.work(ctx); err != nil && !errors.Is(err, context.Canceled) {
				slog.Error("job loop", "error", err)
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
	}
}

func (a *Agent) heartbeats(ctx context.Context) {
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()
	for {
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		stop, err := a.client.Heartbeat(ctx, map[string]any{"goroutines": runtime.NumGoroutine(), "heapBytes": mem.HeapAlloc, "jobsRun": a.jobs.Load()})
		if err != nil {
			slog.Warn("heartbeat failed", "error", err)
		} else {
			if stop && !a.stopped.Load() {
				slog.Warn("emergency stop is on: all discovery halted")
			}
			a.stopped.Store(stop)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func has(methods []string, m string) bool {
	for _, x := range methods {
		if x == m {
			return true
		}
	}
	return false
}

// work claims one job and runs it target by target, posting after each, so a
// pause, stop or emergency stop takes effect between targets.
func (a *Agent) work(ctx context.Context) error {
	job, err := a.client.Claim(ctx)
	if err != nil || job == nil {
		return err
	}
	a.jobs.Add(1)
	slog.Info("job claimed", "job", job.ID, "policy", job.Config.PolicyName, "targets", len(job.Targets), "methods", job.Config.Methods)
	jctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { // the emergency stop cancels work in flight
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-jctx.Done():
				return
			case <-t.C:
				if a.stopped.Load() {
					cancel()
					return
				}
			}
		}
	}()
	for i, target := range job.Targets {
		obs, logs, status := a.scan(jctx, job.Config, target)
		if jctx.Err() != nil {
			_, _ = a.client.Post(ctx, job.ID, Results{Logs: []LogLine{{"warning", "Agent halted mid-target by the emergency stop."}}})
			return errHalt
		}
		progress := (i + 1) * 100 / len(job.Targets)
		res := Results{Targets: []TargetUpdate{{ID: target.ID, Status: status}}, Logs: logs, Progress: progress, Done: i == len(job.Targets)-1}
		for start := 0; start < len(obs) || start == 0; start += 4000 {
			end := start + 4000
			if end > len(obs) {
				end = len(obs)
			}
			batch := res
			batch.Observations = obs[start:end]
			if end < len(obs) {
				batch.Done, batch.Targets = false, nil
			}
			instruction, err := a.client.Post(ctx, job.ID, batch)
			if err != nil {
				return err
			}
			if instruction != "continue" {
				slog.Info("server halted the job", "job", job.ID, "instruction", instruction)
				return errHalt
			}
			if end >= len(obs) {
				break
			}
		}
	}
	slog.Info("job finished", "job", job.ID)
	return nil
}

func (a *Agent) scan(ctx context.Context, cfg JobConfig, t JobTarget) ([]Observation, []LogLine, string) {
	var obs []Observation
	var logs []LogLine
	now := func() time.Time { return time.Now().UTC() }
	hosts, err := Hosts(t.CIDR)
	if err != nil {
		return nil, []LogLine{{"error", err.Error()}}, "failed"
	}
	var alive []netip.Addr
	if has(cfg.Methods, "icmp") {
		if alive, err = Ping(ctx, hosts, cfg); err != nil {
			logs = append(logs, LogLine{"error", "ICMP on " + t.CIDR + ": " + err.Error()})
		}
		for _, h := range alive {
			obs = append(obs, Observation{Method: "icmp", Address: h.String(), Alive: true, ObservedAt: now()})
		}
	}
	if has(cfg.Methods, "nmap") {
		found, err := Nmap(ctx, t.CIDR, cfg)
		if err != nil {
			logs = append(logs, LogLine{"warning", "Nmap on " + t.CIDR + ": " + err.Error()})
		}
		for _, h := range found {
			obs = append(obs, Observation{Method: "nmap", Address: h.Address, MAC: h.MAC, Hostname: h.Hostname, Alive: true,
				Payload: map[string]any{"vendor": h.Vendor}, ObservedAt: now()})
			if a, err := netip.ParseAddr(h.Address); err == nil {
				alive = append(alive, a)
			}
		}
	}
	if has(cfg.Methods, "arp") || has(cfg.Methods, "ndp") {
		p, _ := netip.ParsePrefix(t.CIDR)
		for addr, mac := range Neighbours(ctx) {
			if p.Contains(addr) && ((addr.Is4() && has(cfg.Methods, "arp")) || (addr.Is6() && has(cfg.Methods, "ndp"))) {
				method := "arp"
				if addr.Is6() {
					method = "ndp"
				}
				obs = append(obs, Observation{Method: method, Address: addr.String(), MAC: mac, Alive: true, ObservedAt: now()})
			}
		}
	}
	if has(cfg.Methods, "dns") {
		lookup := alive
		if len(lookup) == 0 {
			lookup = hosts // passive-only jobs ask DNS about every address in the target
		}
		for addr, name := range ReverseDNS(ctx, lookup, cfg.MaxPPS) {
			obs = append(obs, Observation{Method: "dns", Address: addr.String(), Hostname: name, ObservedAt: now(),
				Payload: map[string]any{"ptr": name}})
		}
	}
	if has(cfg.Methods, "snmp") {
		logs = append(logs, LogLine{"warning", "SNMP isn't collected yet: it needs SNMPv3 credentials, which arrive with integration credential storage."})
	}
	logs = append(logs, LogLine{"info", t.CIDR + ": " + itoa(len(obs)) + " results."})
	return obs, logs, "done"
}

func itoa(n int) string {
	const digits = "0123456789"
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{digits[n%10]}, b...)
		n /= 10
	}
	return string(b)
}
