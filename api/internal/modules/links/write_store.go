package links

import (
	"encoding/json"
	"net/http"

	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// loadCurrent locks a saved link and reads what a change is checked against,
// with its stored form for the audit record. A new link (no id) has neither.
// An archived link is refused: it has to be restored before editing.
func loadCurrent(c *gin.Context, tx pgx.Tx, id string) (current, json.RawMessage, error) {
	var was current
	if id == "" {
		return was, nil, nil
	}
	old, err := lockLink(c, tx, id)
	if err != nil {
		return was, nil, err
	}
	if err := tx.QueryRow(c, `SELECT link_type, source_site, COALESCE(provider_id::text, ''), COALESCE(officer_id::text, ''),
		COALESCE(contract_end::text, ''), archived_at IS NOT NULL, device_ids::text[] FROM network_links WHERE id = $1::uuid`, id).
		Scan(&was.Type, &was.Site, &was.ProviderID, &was.OfficerID, &was.ContractEnd, &was.Archived, &was.DeviceIDs); err != nil {
		return was, old, err
	}
	if was.Archived {
		return was, old, httpx.NewError(http.StatusConflict, "LINK_ARCHIVED", "This link is archived. Restore it before editing.")
	}
	return was, old, nil
}

// storeLink inserts a new link or updates a saved one, and returns its ID.
func storeLink(c *gin.Context, tx pgx.Tx, cols []col, id string) (string, error) {
	if id == "" {
		q, args := insertSQL(cols, col{"created_by", "uuid", actor(c)})
		err := tx.QueryRow(c, q, args...).Scan(&id)
		return id, err
	}
	q, args := updateSQL(cols, id)
	_, err := tx.Exec(c, q, args...)
	return id, err
}
