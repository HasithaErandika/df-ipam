package discovery

import (
	"context"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// Observation is one fact an agent saw.
type Observation struct {
	Method     string          `json:"method"`
	Address    string          `json:"address"`
	MAC        string          `json:"mac"`
	Hostname   string          `json:"hostname"`
	Alive      bool            `json:"alive"`
	Payload    json.RawMessage `json:"payload"`
	ObservedAt time.Time       `json:"observedAt"`
}

// ResultBatch is what an agent posts while working through a job.
type ResultBatch struct {
	Observations []Observation `json:"observations"`
	Targets      []struct {
		ID         string `json:"id"`
		Status     string `json:"status"`
		Checkpoint string `json:"checkpoint"`
	} `json:"targets"`
	Logs []struct {
		Level   string `json:"level"`
		Message string `json:"message"`
	} `json:"logs"`
	Progress int    `json:"progress"`
	Done     bool   `json:"done"`
	Error    string `json:"error"`
}

var targetStatuses = map[string]bool{"running": true, "done": true, "failed": true, "skipped": true}

// results records an agent's findings. Each address is checked again against
// the frozen scope and current exclusions; anything outside is dropped and
// logged. Only evidence and observation fields change: never allocation
// state, never ownership. The reply tells the agent to continue, pause or stop.
func (h *Handler) results(c *gin.Context) {
	jobID, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var req ResultBatch
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8<<20)
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	if len(req.Observations) > MaxObservations {
		httpx.Fail(c, httpx.Validation("observations", fmt.Sprintf("Send at most %d observations per call.", MaxObservations)))
		return
	}
	ctx, agent := c.Request.Context(), c.GetString(agentKey)
	var reply resultReply
	err := db.InTx(ctx, h.svc.conn, func(tx pgx.Tx) (err error) {
		reply, err = recordResults(ctx, tx, jobID, agent, req)
		return err
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, reply)
}

func recordResults(ctx context.Context, tx pgx.Tx, jobID, agent string, req ResultBatch) (resultReply, error) {
	reply := resultReply{SkipTargets: []string{}}
	status, windows, err := lockAgentJob(ctx, tx, jobID, agent)
	if err != nil {
		return reply, err
	}
	stopped, err := Stopped(ctx, tx)
	if err != nil {
		return reply, err
	}
	accepted, dropped, err := storeObservations(ctx, tx, jobID, req.Observations)
	if err != nil {
		return reply, err
	}
	if err := updateTargets(ctx, tx, jobID, req); err != nil {
		return reply, err
	}
	if err := writeAgentLogs(ctx, tx, jobID, req, dropped); err != nil {
		return reply, err
	}
	reply.Instruction = instructionFor(status, stopped)
	if status != "running" {
		return reply, nil
	}
	if err := advanceJob(ctx, tx, jobID, req, accepted); err != nil {
		return reply, err
	}
	if req.Done {
		return reply, nil
	}
	return holdToLimits(ctx, tx, jobID, windows, reply)
}

// lockAgentJob locks the job and returns its status. A job another agent owns
// looks the same as one that doesn't exist.
func lockAgentJob(ctx context.Context, tx pgx.Tx, jobID, agent string) (string, []Window, error) {
	var status string
	var owner *string
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT status, agent_id::text, config_snapshot FROM discovery_jobs WHERE id = $1::uuid FOR UPDATE`, jobID).Scan(&status, &owner, &raw)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (owner == nil || *owner != agent)) {
		return "", nil, httpx.ErrNotFound
	}
	if err != nil {
		return "", nil, err
	}
	var snap Snapshot
	_ = json.Unmarshal(raw, &snap)
	return status, snap.Windows, nil
}

func instructionFor(status string, stopped bool) string {
	switch {
	case status == "paused":
		return "pause"
	case status != "running" || stopped:
		return "stop"
	}
	return "continue"
}

func updateTargets(ctx context.Context, tx pgx.Tx, jobID string, req ResultBatch) error {
	// The last report for a target wins, as a single UPDATE can't apply two.
	latest := map[string]int{}
	var ids, statuses, checkpoints []string
	for _, t := range req.Targets {
		if !httpx.IsUUID(t.ID) || !targetStatuses[t.Status] {
			continue
		}
		i, seen := latest[t.ID]
		if !seen {
			i = len(ids)
			latest[t.ID] = i
			ids, statuses, checkpoints = append(ids, t.ID), append(statuses, ""), append(checkpoints, "")
		}
		statuses[i], checkpoints[i] = t.Status, truncate(t.Checkpoint, 200)
	}
	if len(ids) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `UPDATE discovery_job_targets d SET status = t.status, checkpoint = NULLIF(t.checkpoint, '')
		FROM unnest($2::uuid[], $3::text[], $4::text[]) AS t(id, status, checkpoint)
		WHERE d.id = t.id AND d.job_id = $1::uuid`, jobID, ids, statuses, checkpoints)
	return err
}

func writeAgentLogs(ctx context.Context, tx pgx.Tx, jobID string, req ResultBatch, dropped int) error {
	lines := make([]LogLine, 0, len(req.Logs)+1)
	for _, l := range req.Logs {
		level := l.Level
		if level != "info" && level != "warning" && level != "error" {
			level = "info"
		}
		lines = append(lines, LogLine{level, l.Message})
	}
	if dropped > 0 {
		lines = append(lines, LogLine{"warning", fmt.Sprintf("Dropped %d results outside the job's approved scope or with an unknown method.", dropped)})
	}
	return LogLines(ctx, tx, jobID, lines)
}

// advanceJob records progress, or the end of the job when the agent says it's done.
func advanceJob(ctx context.Context, tx pgx.Tx, jobID string, req ResultBatch, accepted int) error {
	progress := req.Progress
	if progress < 0 || progress > 100 {
		progress = 0
	}
	var err error
	switch {
	case req.Done && req.Error != "":
		_, err = tx.Exec(ctx, `UPDATE discovery_jobs SET status = 'failed', progress = $2, error = $3, finished_at = now() WHERE id = $1::uuid`, jobID, progress, truncate(req.Error, 2000))
	case req.Done:
		_, err = tx.Exec(ctx, `UPDATE discovery_jobs SET status = 'completed', progress = 100, finished_at = now() WHERE id = $1::uuid`, jobID)
	default:
		_, err = tx.Exec(ctx, `UPDATE discovery_jobs SET progress = $2 WHERE id = $1::uuid`, jobID, progress)
	}
	if err != nil || !req.Done {
		return err
	}
	return Log(ctx, tx, jobID, "info", fmt.Sprintf("Agent finished. %d results accepted in the last batch.", accepted))
}
