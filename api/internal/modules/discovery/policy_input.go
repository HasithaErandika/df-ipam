package discovery

import (
	"dfipam/api/internal/platform/httpx"
	"net/netip"
	"strings"
)

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
	if err := in.checkTargets(); err != nil {
		return err
	}
	if err := in.checkLimits(); err != nil {
		return err
	}
	if in.Windows == nil {
		in.Windows = []Window{}
	}
	if err := ValidateWindows(in.Windows); err != nil {
		return httpx.Validation("windows", err.Error())
	}
	return nil
}

func (in *PolicyInput) checkTargets() error {
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
	return nil
}

func withDefault(v *int, fallback int) {
	if *v == 0 {
		*v = fallback
	}
}

func (in *PolicyInput) checkLimits() error {
	withDefault(&in.MaxConcurrency, 16)
	withDefault(&in.MaxPPS, 50)
	withDefault(&in.TimeoutMS, 1000)
	if in.MaxConcurrency < 1 || in.MaxConcurrency > 64 || in.MaxPPS < 1 || in.MaxPPS > hardMaxTotalPPS || in.TimeoutMS < 100 || in.TimeoutMS > 30000 || in.Retries < 0 || in.Retries > 5 {
		return httpx.Validation("maxPps", "Limits: concurrency 1-64, packets per second 1-500, timeout 100-30000 ms, retries 0-5.")
	}
	return nil
}
