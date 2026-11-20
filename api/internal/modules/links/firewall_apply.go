package links

import (
	"net/http"
	"slices"
	"sort"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// applyItem is one confirmed proposal: add it with the business details people
// gave, or update the listed fields of the link already recorded.
type applyItem struct {
	Key         string   `json:"key"`
	Action      string   `json:"action"` // add, update
	Name        string   `json:"name"`
	LinkType    string   `json:"linkType"`
	SourceSite  string   `json:"sourceSite"`
	Destination string   `json:"destination"`
	Purpose     string   `json:"purpose"`
	LinkRole    string   `json:"linkRole"`
	Criticality string   `json:"criticality"`
	Status      string   `json:"status"`
	ProviderID  string   `json:"providerId"`
	OfficerID   string   `json:"officerId"`
	Fields      []string `json:"fields"`
}

type applyResult struct {
	Key    string `json:"key"`
	Action string `json:"action"`
	OK     bool   `json:"ok"`
	LinkID string `json:"linkId,omitempty"`
	Code   string `json:"code,omitempty"`
	Error  string `json:"error,omitempty"`
}

// firewallApply reads the firewall again, so nothing is saved from a stale
// review, then adds or updates each confirmed item in its own savepoint.
// Interfaces go before tunnels, so a tunnel can name the line it runs over.
func (h *Handler) firewallApply(c *gin.Context) {
	var in struct {
		Items []applyItem `json:"items"`
	}
	if !httpx.Bind(c, &in) {
		return
	}
	if len(in.Items) == 0 || len(in.Items) > 200 {
		httpx.Fail(c, httpx.Validation("items", "Tick from 1 to 200 items to apply."))
		return
	}
	client, ok := h.firewallClient(c)
	if !ok {
		return
	}
	r, err := buildReview(c, h.conn, client)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	s, err := loadSettings(c, h.conn)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	sort.SliceStable(in.Items, func(i, j int) bool { return isInterfaceKey(in.Items[i].Key) && !isInterfaceKey(in.Items[j].Key) })
	run := &applyRun{c: c, s: s, firewall: r.Firewall, byKey: map[string]proposal{}, made: map[string]string{}}
	for _, p := range r.Proposals {
		run.byKey[p.Key] = p
		if p.Existing != nil {
			run.made[p.Key] = p.Existing.ID
		}
	}
	results := make([]applyResult, 0, len(in.Items))
	err = db.InTx(c, h.conn, func(tx pgx.Tx) error {
		for _, item := range in.Items {
			results = append(results, run.one(tx, item))
		}
		return nil
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, gin.H{"firewall": r.Firewall, "results": results})
}

func isInterfaceKey(k string) bool { return len(k) > 10 && k[:10] == "interface:" }

type applyRun struct {
	c        *gin.Context
	s        Settings
	firewall string
	byKey    map[string]proposal
	made     map[string]string // proposal key -> link id, for underlying links
	names    *resolver         // read once, for every update in this apply
}

func (r *applyRun) one(tx pgx.Tx, item applyItem) applyResult {
	res := applyResult{Key: item.Key, Action: item.Action}
	p, ok := r.byKey[item.Key]
	in, id, err := r.prepare(tx, item, p, ok)
	if err == nil {
		res.LinkID, err = r.save(tx, in, id, item)
	}
	if err != nil {
		res.Error = describe(err)
		return res
	}
	res.OK = true
	r.made[item.Key] = res.LinkID
	_ = tx.QueryRow(r.c, `SELECT code FROM network_links WHERE id = $1::uuid`, res.LinkID).Scan(&res.Code)
	return res
}

// prepare builds the form for an item, checking it is still on the firewall
// and that the person may add or change it.
func (r *applyRun) prepare(tx pgx.Tx, item applyItem, p proposal, found bool) (*linkInput, string, error) {
	switch {
	case !found:
		return nil, "", rowError("It is no longer on the firewall. Fetch again.")
	case item.Action == "add" && p.Existing != nil:
		return nil, "", rowError("It is already recorded as " + p.Existing.Code + ". Update it instead.")
	case item.Action == "add" && !can(r.c, auth.LinksCreate), item.Action == "update" && !can(r.c, auth.LinksEdit):
		return nil, "", httpx.ErrForbidden
	case item.Action == "add":
		return r.newInput(item, p), "", nil
	case item.Action == "update" && p.Existing != nil:
		in, err := r.current(tx, p.Existing.ID)
		if err != nil {
			return nil, "", err
		}
		for _, f := range item.Fields {
			if !slices.Contains(updatable, f) {
				return nil, "", rowError("The firewall can't update " + f + ".")
			}
			setFromFirewall(in, f, p.Suggestion)
		}
		return in, p.Existing.ID, nil
	}
	return nil, "", rowError("Choose to add it, or to update the link already recorded.")
}

func (r *applyRun) save(tx pgx.Tx, in *linkInput, id string, item applyItem) (string, error) {
	sp, err := tx.Begin(r.c)
	if err != nil {
		return "", err
	}
	t, _ := r.s.typeOf(in.Type)
	if err = in.validate(t.VPN); err == nil {
		// Commercial details never come from the firewall, so they are left alone.
		id, err = writeLink(r.c, sp, in, id, r.s, false)
	}
	if err == nil {
		err = record(r.c, sp, "from_firewall", "network_link", id, nil, map[string]any{"firewall": r.firewall, "item": item.Key, "action": item.Action, "fields": item.Fields})
	}
	if err != nil {
		_ = sp.Rollback(r.c)
		return "", err
	}
	return id, sp.Commit(r.c)
}
