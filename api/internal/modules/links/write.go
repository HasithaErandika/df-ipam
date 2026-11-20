package links

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// current is what a saved link already holds that a change is checked against.
type current struct {
	Type, Site, ProviderID, OfficerID, ContractEnd string
	Archived                                       bool
	DeviceIDs                                      []string
}

func (h *Handler) create(c *gin.Context) { h.save(c, "") }

func (h *Handler) update(c *gin.Context) {
	if id, ok := httpx.PathID(c); ok {
		h.save(c, id)
	}
}

// save adds or edits a link with its related links and local subnets, all in
// one transaction with its audit record. Without links:commercial_manage the
// commercial fields are left as they are.
func (h *Handler) save(c *gin.Context, id string) {
	var in linkInput
	if !httpx.Bind(c, &in) {
		return
	}
	settings, err := loadSettings(c, h.conn)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	t, _ := settings.typeOf(in.Type)
	if err := in.validate(t.VPN); err != nil {
		httpx.Fail(c, err)
		return
	}
	money := can(c, auth.LinksCommercialManage)
	err = db.InTx(c, h.conn, func(tx pgx.Tx) error {
		var err error
		id, err = writeLink(c, tx, &in, id, settings, money)
		return err
	})
	if err != nil {
		httpx.Fail(c, db.Friendly(err, "Another link from this provider already has this circuit ID."))
		return
	}
	status := http.StatusOK
	if c.Request.Method == http.MethodPost {
		status = http.StatusCreated
	}
	httpx.OK(c, status, gin.H{"id": id})
}

// writeLink stores one link inside tx; the import uses it too.
func writeLink(c *gin.Context, tx pgx.Tx, in *linkInput, id string, s Settings, money bool) (string, error) {
	var was current
	var old json.RawMessage
	if id != "" {
		raw, err := lockLink(c, tx, id)
		if err != nil {
			return id, err
		}
		old = raw
		if err := tx.QueryRow(c, `SELECT link_type, source_site, COALESCE(provider_id::text, ''), COALESCE(officer_id::text, ''),
			COALESCE(contract_end::text, ''), archived_at IS NOT NULL, device_ids::text[] FROM network_links WHERE id = $1::uuid`, id).
			Scan(&was.Type, &was.Site, &was.ProviderID, &was.OfficerID, &was.ContractEnd, &was.Archived, &was.DeviceIDs); err != nil {
			return id, err
		}
		if was.Archived {
			return id, httpx.NewError(http.StatusConflict, "LINK_ARCHIVED", "This link is archived. Restore it before editing.")
		}
	}
	if err := checkChoices(c, tx, in, was, s); err != nil {
		return id, err
	}
	if err := checkDevices(c, tx, in.DeviceIDs, was.DeviceIDs); err != nil {
		return id, err
	}
	cols := in.baseCols()
	if money {
		cols = append(cols, in.commercialCols()...)
	}
	if id == "" {
		q, args := insertSQL(cols, col{"created_by", "uuid", actor(c)})
		if err := tx.QueryRow(c, q, args...).Scan(&id); err != nil {
			return id, err
		}
	} else {
		q, args := updateSQL(cols, id)
		if _, err := tx.Exec(c, q, args...); err != nil {
			return id, err
		}
	}
	if err := saveRelations(c, tx, id, in); err != nil {
		return id, err
	}
	return id, auditSave(c, tx, id, old, in, was, money)
}

// auditSave records the change, and a renewal when the contract end moves later.
// A new contract end starts its reminders again.
func auditSave(c *gin.Context, tx pgx.Tx, id string, old json.RawMessage, in *linkInput, was current, money bool) error {
	now, err := storedLink(c, tx, id)
	if err != nil {
		return err
	}
	if old == nil {
		if err := record(c, tx, "create", "network_link", id, nil, now); err != nil {
			return err
		}
	} else if err := record(c, tx, "update", "network_link", id, old, now); err != nil {
		return err
	}
	if !money || old == nil || in.ContractEnd == was.ContractEnd {
		return nil
	}
	if _, err := tx.Exec(c, `UPDATE network_links SET reminders_sent = '{}' WHERE id = $1::uuid`, id); err != nil {
		return err
	}
	if was.ContractEnd != "" && in.ContractEnd > was.ContractEnd {
		return record(c, tx, "renew", "network_link", id, map[string]string{"contractEnd": was.ContractEnd}, map[string]string{"contractEnd": in.ContractEnd})
	}
	return nil
}

// checkChoices refuses switched-off types, unknown sites, inactive providers
// and people, unless the link already had them.
func checkChoices(ctx context.Context, tx pgx.Tx, in *linkInput, was current, s Settings) error {
	if t, ok := s.typeOf(in.Type); !ok || (!t.Active && in.Type != was.Type) {
		return httpx.Validation("linkType", "Choose a listed link type.")
	}
	known := slices.ContainsFunc(s.Sites, func(x string) bool { return strings.EqualFold(x, in.SourceSite) })
	if !known && !strings.EqualFold(in.SourceSite, was.Site) {
		return httpx.Validation("sourceSite", "Choose a listed location. Administrators add them in Network setup.")
	}
	if in.ProviderID != "" && in.ProviderID != was.ProviderID {
		var ok bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM network_link_providers WHERE id = $1::uuid AND is_active)`, in.ProviderID).Scan(&ok); err != nil || !ok {
			return httpx.Validation("providerId", "Choose an active provider.")
		}
	}
	if in.OfficerID != "" && in.OfficerID != was.OfficerID {
		var ok bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE id = $1::uuid AND is_active)`, in.OfficerID).Scan(&ok); err != nil || !ok {
			return httpx.Validation("officerId", "Choose an active user.")
		}
	}
	return nil
}

// saveRelations replaces the link's related links and local subnets. A pair is
// stored once: a link can't relate to itself, nor to a link that already
// relates to it.
func saveRelations(ctx context.Context, tx pgx.Tx, id string, in *linkInput) error {
	related, kinds := make([]string, len(in.Relations)), make([]string, len(in.Relations))
	for i, r := range in.Relations {
		if r.RelatedID == id {
			return httpx.Validation("relations", "A link can't be its own backup or underlying link.")
		}
		related[i], kinds[i] = r.RelatedID, r.Kind
	}
	var clash string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(min(l.code), '') FROM network_link_relations r JOIN network_links l ON l.id = r.link_id
		WHERE r.related_id = $1::uuid AND r.link_id = ANY($2::uuid[])`, id, related).Scan(&clash); err != nil {
		return err
	}
	if clash != "" {
		return httpx.Validation("relations", clash+" already lists this link as related. Keep the relationship on one side only.")
	}
	if _, err := tx.Exec(ctx, `DELETE FROM network_link_relations WHERE link_id = $1::uuid`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO network_link_relations (link_id, related_id, kind)
		SELECT $1::uuid, r, k FROM unnest($2::uuid[], $3::text[]) AS x(r, k)`, id, related, kinds); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM network_link_segments WHERE link_id = $1::uuid`, id); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO network_link_segments (link_id, segment_id) SELECT DISTINCT $1::uuid, s FROM unnest($2::uuid[]) s`,
		id, nonNil(in.LocalSegmentIDs))
	return err
}
