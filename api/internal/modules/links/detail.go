package links

import (
	"net/http"
	"time"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type relatedDTO struct {
	ID     string `db:"id" json:"id"`
	Code   string `db:"code" json:"code"`
	Name   string `db:"name" json:"name"`
	Type   string `db:"link_type" json:"linkType"`
	Role   string `db:"link_role" json:"linkRole"`
	Status string `db:"status" json:"status"`
	Kind   string `db:"kind" json:"kind"`
	// Outgoing: this link lists it. Incoming: it lists this link.
	Direction string `db:"direction" json:"direction"`
}

type segmentRef struct {
	ID      string `db:"id" json:"id"`
	Name    string `db:"name" json:"name"`
	Network string `db:"network" json:"network"`
}

type registerRef struct {
	ID          string `db:"id" json:"id"`
	IP          string `db:"ip" json:"ip"`
	Name        string `db:"name" json:"name"`
	SegmentID   string `db:"segment_id" json:"segmentId"`
	SegmentName string `db:"segment_name" json:"segmentName"`
	Use         string `db:"use" json:"use"`
}

type attachmentDTO struct {
	ID          string     `db:"id" json:"id"`
	Category    string     `db:"category" json:"category"`
	FileName    string     `db:"file_name" json:"fileName"`
	ContentType string     `db:"content_type" json:"contentType"`
	Size        int64      `db:"size_bytes" json:"size"`
	UploadedBy  *string    `db:"uploaded_by" json:"uploadedBy"`
	UploadedAt  time.Time  `db:"uploaded_at" json:"uploadedAt"`
	RemovedAt   *time.Time `db:"removed_at" json:"removedAt"`
}

// commercialFiles are attachment categories only commercial viewers may open.
var commercialFiles = []string{"contract", "quotation"}

func (h *Handler) detail(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	links, err := db.All[linkDTO](c, h.conn, linkColumns+linkFrom+" WHERE l.id = $1::uuid", id)
	if err == nil && len(links) == 0 {
		err = httpx.ErrNotFound
	}
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	money := can(c, auth.LinksCommercialView)
	link := links[0]
	if !money {
		link.hideCommercial()
	}
	seen, err := devicesSeenAt(c, h.conn, linkAddresses(&link))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out := gin.H{"link": link, "seenDevices": seen}
	if err := h.attachWAN(c, id, out); err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.attachRefs(c, id, money, out); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

// attachRefs adds the related links, local subnets, register entries and files.
func (h *Handler) attachRefs(c *gin.Context, id string, money bool, out gin.H) error {
	related, err := db.All[relatedDTO](c, h.conn, `
		SELECT l.id::text AS id, l.code, l.name, l.link_type, l.link_role, l.status, r.kind, 'outgoing' AS direction
		FROM network_link_relations r JOIN network_links l ON l.id = r.related_id WHERE r.link_id = $1::uuid
		UNION ALL
		SELECT l.id::text, l.code, l.name, l.link_type, l.link_role, l.status, r.kind, 'incoming'
		FROM network_link_relations r JOIN network_links l ON l.id = r.link_id WHERE r.related_id = $1::uuid
		ORDER BY 8 DESC, 2`, id)
	if err != nil {
		return err
	}
	segments, err := db.All[segmentRef](c, h.conn, `SELECT s.id::text AS id, s.name, s.network::text AS network
		FROM network_link_segments x JOIN segments s ON s.id = x.segment_id WHERE x.link_id = $1::uuid ORDER BY s.network`, id)
	if err != nil {
		return err
	}
	register, err := db.All[registerRef](c, h.conn, `SELECT i.id::text AS id, host(i.ip) AS ip, i.assigned_name AS name, s.id::text AS segment_id,
		s.name AS segment_name, CASE WHEN i.ip = l.wan_ip OR i.ip = ANY(x.extra) THEN 'wan' WHEN i.ip = l.gateway_ip THEN 'gateway' ELSE 'public' END AS use
		FROM network_links l CROSS JOIN LATERAL (SELECT array(SELECT set_masklen(e, 32) FROM unnest(l.extra_wan_ips) e) AS extra) x
		JOIN ip_addresses i ON i.ip = l.wan_ip OR i.ip = ANY(x.extra) OR i.ip = l.gateway_ip OR l.public_ips >>= i.ip
		JOIN segments s ON s.id = i.segment_id WHERE l.id = $1::uuid ORDER BY i.ip LIMIT 50`, id)
	if err != nil {
		return err
	}
	files, err := db.All[attachmentDTO](c, h.conn, `SELECT a.id::text AS id, a.category, a.file_name, a.content_type, a.size_bytes,
		u.email AS uploaded_by, a.uploaded_at, a.removed_at FROM network_link_attachments a LEFT JOIN users u ON u.id = a.uploaded_by
		WHERE a.link_id = $1::uuid AND (a.removed_at IS NULL OR $2) AND ($3 OR NOT (a.category = ANY($4::text[])))
		ORDER BY a.removed_at IS NOT NULL, a.uploaded_at DESC`, id, can(c, auth.AuditView), money, commercialFiles)
	if err != nil {
		return err
	}
	out["related"], out["localSegments"], out["registerEntries"], out["attachments"] = related, segments, register, files
	return nil
}

func (h *Handler) archive(c *gin.Context) { h.setArchived(c, true) }

func (h *Handler) restore(c *gin.Context) { h.setArchived(c, false) }

// setArchived archives a link or brings it back. Archived links leave the
// lists, totals and reminders but keep their history.
func (h *Handler) setArchived(c *gin.Context, archive bool) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		old, err := lockLink(c, tx, id)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(c, `UPDATE network_links SET archived_at = CASE WHEN $2 THEN now() END, archived_by = CASE WHEN $2 THEN $3::uuid END
			WHERE id = $1::uuid AND (archived_at IS NULL) = $2`, id, archive, actor(c))
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return httpx.NewError(http.StatusConflict, "ALREADY_DONE", "This link changed while you were working. Reload and try again.")
		}
		action := map[bool]string{true: "archive", false: "restore"}[archive]
		return record(c, tx, action, "network_link", id, old, map[string]bool{"archived": archive})
	})
	if err != nil {
		httpx.Fail(c, db.Friendly(err))
		return
	}
	c.Status(http.StatusNoContent)
}
