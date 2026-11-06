package ipaddresses

import (
	"context"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

func (h *Handler) history(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	rows, err := h.conn.Query(c.Request.Context(), `SELECT a.action, a.occurred_at, u.email, a.result FROM audit_logs a LEFT JOIN users u ON u.id = a.actor_id
		WHERE a.object_type = 'ip_address' AND a.object_id = $1 ORDER BY a.occurred_at DESC, a.id DESC LIMIT 100`, id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	type change struct {
		Action     string    `json:"action"`
		OccurredAt time.Time `json:"occurredAt"`
		Actor      *string   `json:"actorEmail"`
		Result     string    `json:"result"`
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (change, error) {
		var e change
		return e, r.Scan(&e.Action, &e.OccurredAt, &e.Actor, &e.Result)
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, append([]change{}, out...))
}

func (h *Handler) evidence(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	rows, err := h.conn.Query(c.Request.Context(), `
		SELECT os.name, o.mac::text, o.hostname, o.payload, o.observed_at
		FROM ip_addresses i JOIN discovery_observations o ON o.address = i.address AND o.routing_domain_id = i.routing_domain_id
		JOIN observation_sources os ON os.id = o.source_id
		WHERE i.id = $1::uuid ORDER BY o.observed_at DESC LIMIT 100`, id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	type item struct {
		Source     string          `json:"source"`
		MAC        *string         `json:"mac"`
		Hostname   *string         `json:"hostname"`
		Payload    json.RawMessage `json:"payload"`
		ObservedAt time.Time       `json:"observedAt"`
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (item, error) {
		var e item
		return e, r.Scan(&e.Source, &e.MAC, &e.Hostname, &e.Payload, &e.ObservedAt)
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, append([]item{}, out...))
}

// ChangeFields lists what a change request may set on an address. Allocation state
// isn't here: it moves only through the allocation workflow.
var ChangeFields = []db.Field{
	{Name: "hostname", Column: "hostname"}, {Name: "notes", Column: "notes"},
	{Name: "mac", Column: "mac", Cast: "macaddr", Nullable: true},
	{Name: "deviceId", Column: "device_id", Cast: "uuid", Nullable: true},
	{Name: "applicationId", Column: "application_id", Cast: "uuid", Nullable: true},
	{Name: "businessServiceId", Column: "business_service_id", Cast: "uuid", Nullable: true},
	{Name: "ownerId", Column: "owner_id", Cast: "uuid", Nullable: true},
}

// ApplyChange applies an approved change request.
func ApplyChange(ctx context.Context, tx pgx.Tx, id, actor string, changes map[string]string) (map[string]string, map[string]string, error) {
	return db.ApplyFields(ctx, tx, "ip_addresses", id, actor, "", ChangeFields, changes)
}
