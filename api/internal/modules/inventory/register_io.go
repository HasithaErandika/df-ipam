package inventory

import (
	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/platform/tabular"

	"github.com/gin-gonic/gin"
)

var registerHeader = []string{"ip", "segment", "status", "ipType", "environment", "assignedName", "platform", "services", "expectedMac", "owner", "department", "monitor", "critical", "checkPort", "health", "notes"}

func (h *Handler) exportRegister(c *gin.Context) {
	format, ok := exportFormat(c)
	if !ok {
		return
	}
	w, err := registerFilter(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := tabular.NewWriter(format, "IP register", registerHeader)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	rows, err := h.conn.Query(c, `SELECT host(i.ip), s.name, i.status, i.ip_type, COALESCE(e.name, ''), i.assigned_name, i.platform, array_to_string(i.services, ' '), COALESCE(i.expected_mac::text, ''),
		i.owner, i.department, i.monitor::text, i.critical::text, COALESCE(i.check_port::text, ''), i.health, i.notes`+ipFrom+w.SQL()+" ORDER BY i.ip", w.Args...)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		r := make([]string, len(registerHeader))
		dest := make([]any, len(r))
		for i := range r {
			dest[i] = &r[i]
		}
		if err := rows.Scan(dest...); err != nil {
			httpx.Fail(c, err)
			return
		}
		_ = out.Write(r)
	}
	if err := rows.Err(); err != nil {
		httpx.Fail(c, err)
		return
	}
	sendTable(c, out, format, "ip-register")
}
