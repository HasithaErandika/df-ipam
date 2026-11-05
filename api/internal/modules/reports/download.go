package reports

import (
	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// download sends a finished report to the person who requested it. A report holds a
// map of the network, so sharing it means asking the system for your own copy.
func (h *Handler) download(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	var kind, format, status string
	var content []byte
	var finished *time.Time
	err := h.conn.QueryRow(ctx, `SELECT report_type, format, status, content, finished_at FROM report_jobs WHERE id = $1::uuid AND requested_by = $2::uuid`, id, actor).
		Scan(&kind, &format, &status, &content, &finished)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(c, httpx.ErrNotFound)
		return
	}
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if status != "completed" || content == nil {
		httpx.Fail(c, errNotDone)
		return
	}
	// Rights can be withdrawn after a report is generated.
	d, known := definition(kind)
	if !known {
		httpx.Fail(c, httpx.ErrNotFound)
		return
	}
	if _, ok := allowed(auth.Current(c), d); !ok {
		httpx.Fail(c, httpx.ErrForbidden)
		return
	}
	err = db.InTx(ctx, h.conn, func(tx pgx.Tx) error {
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "report.download", ObjectType: "report", ObjectID: id, New: gin.H{"reportType": kind, "format": format}})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	f := formats[format]
	date := time.Now().UTC().Format("2006-01-02")
	if finished != nil {
		date = finished.UTC().Format("2006-01-02")
	}
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="df-ipam-%s-%s.%s"`, strings.ReplaceAll(kind, "_", "-"), date, f.extension))
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, f.contentType, content)
}
