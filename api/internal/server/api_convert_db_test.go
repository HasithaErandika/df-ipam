package server

import (
	"context"
	"testing"
	"time"

	"dfipam/api/internal/auth"
)

// requesterSessions signs every request in as one person with the starting
// Requester role's permissions.
type requesterSessions struct{ id string }

func (s requesterSessions) Resolve(context.Context, string) (*auth.Principal, error) {
	return auth.NewPrincipal(s.id, "requester@example.com", "requester",
		[]string{auth.InventoryView, auth.IPRequest, auth.IPReserve, auth.ReportsExport}, false), nil
}

// A Requester can hold an address but can't turn the hold into an assignment:
// that would skip the IP request an approver decides.
func TestRequesterCantConvertTheirHold(t *testing.T) {
	e := newAPI(t)
	seg := id(e.call("POST", "/segments", map[string]any{"name": "Requester lab", "kind": "server", "network": "172.16.77.0/24"}, 200))
	var uid string
	if err := e.tx.QueryRow(e.ctx, `INSERT INTO users (email, auth_provider, auth_subject, role) VALUES ('requester@example.com', 'test', 'req', 'requester') RETURNING id`).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	r, _ := New(Deps{Conn: e.tx, Sessions: requesterSessions{uid}})
	req := &apiEnv{t: e.t, ctx: e.ctx, tx: e.tx, router: r}
	hold := id(req.call("POST", "/reservations", map[string]any{"segmentId": seg, "startIp": "172.16.77.50", "reason": "Hold", "expiresOn": time.Now().AddDate(0, 1, 0).Format("2006-01-02")}, 201))
	req.call("POST", "/reservations/"+hold+"/convert", map[string]any{"ip": "172.16.77.50", "name": "self-assigned"}, 403)
	var n int
	if err := e.tx.QueryRow(e.ctx, "SELECT count(*) FROM ip_addresses WHERE ip = '172.16.77.50'").Scan(&n); err != nil || n != 0 {
		t.Fatalf("want no register entry, got %d (%v)", n, err)
	}
}
