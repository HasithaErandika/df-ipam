package agent

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// Window and the rest mirror the server's frozen job configuration.
type JobConfig struct {
	PolicyName     string   `json:"policyName"`
	Methods        []string `json:"methods"`
	MaxConcurrency int      `json:"maxConcurrency"`
	MaxPPS         int      `json:"maxPps"`
	TimeoutMS      int      `json:"timeoutMs"`
	Retries        int      `json:"retries"`
}

type JobTarget struct {
	ID              string `json:"id"`
	RoutingDomainID string `json:"routingDomainId"`
	CIDR            string `json:"cidr"`
	Status          string `json:"status"`
}

type Job struct {
	ID      string      `json:"jobId"`
	Config  JobConfig   `json:"config"`
	Targets []JobTarget `json:"targets"`
}

// Claim returns the next job, or nil when there is nothing to do.
func (c *Client) Claim(ctx context.Context) (*Job, error) {
	var job Job
	status, err := c.call(ctx, http.MethodGet, "/jobs", nil, &job)
	if err != nil || status == http.StatusNoContent {
		return nil, err
	}
	return &job, nil
}

type Observation struct {
	Method     string         `json:"method"`
	Address    string         `json:"address"`
	MAC        string         `json:"mac,omitempty"`
	Hostname   string         `json:"hostname,omitempty"`
	Alive      bool           `json:"alive"`
	Payload    map[string]any `json:"payload,omitempty"`
	ObservedAt time.Time      `json:"observedAt"`
}

type TargetUpdate struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	Checkpoint string `json:"checkpoint,omitempty"`
}

type LogLine struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

type Results struct {
	Observations []Observation  `json:"observations"`
	Targets      []TargetUpdate `json:"targets,omitempty"`
	Logs         []LogLine      `json:"logs,omitempty"`
	Progress     int            `json:"progress"`
	Done         bool           `json:"done"`
	Error        string         `json:"error,omitempty"`
}

// Post sends results and returns the server's instruction: continue, pause or stop.
// Post sends results. The reply says whether to go on, and which targets the server
// has since closed, because of a new exclusion or blackout.
func (c *Client) Post(ctx context.Context, jobID string, r Results) (string, []string, error) {
	if r.Observations == nil {
		r.Observations = []Observation{}
	}
	var out struct {
		Instruction string   `json:"instruction"`
		SkipTargets []string `json:"skipTargets"`
	}
	_, err := c.call(ctx, http.MethodPost, "/jobs/"+url.PathEscape(jobID)+"/results", r, &out)
	return out.Instruction, out.SkipTargets, err
}
