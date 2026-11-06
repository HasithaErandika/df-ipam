package imports

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"dfipam/api/internal/modules/catalog"
	"dfipam/api/internal/modules/ipaddresses"
	"dfipam/api/internal/modules/organisations"
	"dfipam/api/internal/modules/routingdomains"
	"dfipam/api/internal/modules/subnets"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/jackc/pgx/v5"
)

// lineError stops the whole import and names the row, so nothing is half done.
func lineError(s *sheet, row int, err error) error {
	var e *httpx.Error
	switch {
	case errors.As(err, &e):
		message := e.Message
		if len(e.Details) > 0 {
			message = e.Details[0].Field + ": " + e.Details[0].Issue
		}
		return httpx.NewError(http.StatusConflict, "IMPORT_LINE_FAILED", fmt.Sprintf("%s row %d: %s Nothing was imported.", s.Title, row, message))
	case db.ErrorCode(err) == db.CodeUnique:
		return httpx.NewError(http.StatusConflict, "IMPORT_LINE_FAILED", fmt.Sprintf("%s row %d now matches an existing record. Nothing was imported.", s.Title, row))
	}
	return err
}

// body turns a row into what the module's own create takes.
func body(s *sheet, fields map[string]string, refs map[string]*entry) map[string]any {
	out := map[string]any{}
	for i := range s.Columns {
		c := &s.Columns[i]
		v, present := fields[c.Name]
		switch {
		case !present || c.Field == "":
		case c.Ref != "":
			out[c.Field] = refs[c.Name].id
		case c.Kind == intCell:
			n, _ := strconv.Atoi(v)
			out[c.Field] = float64(n)
		case c.Kind == boolCell:
			out[c.Field] = v == "true"
		default:
			out[c.Field] = v
		}
	}
	return out
}

func decode[T any](b map[string]any) (T, error) {
	var out T
	raw, err := json.Marshal(b)
	if err != nil {
		return out, err
	}
	return out, json.Unmarshal(raw, &out)
}

func writeRow(ctx context.Context, tx pgx.Tx, actor string, s *sheet, b map[string]any) (string, error) {
	switch s.Key {
	case "organisations":
		o, err := organisations.Insert(ctx, tx, actor, b["name"].(string), strings.ToUpper(b["code"].(string)))
		return o.ID, err
	case "routing_domains":
		req, err := decode[routingdomains.NewDomain](b)
		if err != nil {
			return "", err
		}
		return routingdomains.Insert(ctx, tx, actor, req)
	case "subnets":
		req, err := decode[subnets.CreateRequest](b)
		if err != nil {
			return "", err
		}
		out, err := subnets.NewService(tx).Create(ctx, actor, req)
		return out.ID, err
	}
	req, err := decode[ipaddresses.NewAddress](b)
	if err != nil {
		return "", err
	}
	return ipaddresses.Insert(ctx, tx, actor, req)
}

// writeSheet creates a sheet's rows. Catalog rows go in one round trip. The rest go
// through their module one at a time, because each has rules of its own to apply.
func writeSheet(ctx context.Context, tx pgx.Tx, actor string, s *sheet, group []pendingRecord, bodies []map[string]any) ([]string, error) {
	if e, ok := catalog.ByPath(s.Entity); ok {
		ids, err := catalog.InsertMany(ctx, tx, actor, e, bodies)
		var bulk *catalog.BulkError
		if errors.As(err, &bulk) {
			return nil, lineError(s, group[bulk.Index].parsed.Row, bulk.Err)
		}
		return ids, err
	}
	ids := make([]string, len(bodies))
	for i, b := range bodies {
		id, err := writeRow(ctx, tx, actor, s, b)
		if err != nil {
			return nil, lineError(s, group[i].parsed.Row, err)
		}
		ids[i] = id
	}
	return ids, nil
}

// nestBlocks puts each new block under the smallest block that contains it.
func nestBlocks(ctx context.Context, tx pgx.Tx, ids []string) error {
	_, err := tx.Exec(ctx, `UPDATE network_blocks b SET parent_id = (SELECT p.id FROM network_blocks p
		WHERE p.routing_domain_id = b.routing_domain_id AND p.cidr >> b.cidr ORDER BY masklen(p.cidr) DESC LIMIT 1)
		WHERE b.id = ANY($1::uuid[])`, ids)
	return err
}

func sheetBodies(ctx context.Context, x *index, s *sheet, group []pendingRecord) ([]map[string]any, error) {
	rows := make([]*sheetRow, len(group))
	for i := range group {
		rows[i] = &sheetRow{fields: group[i].parsed.Fields}
	}
	if err := loadRefs(ctx, x, s, rows); err != nil {
		return nil, err
	}
	bodies := make([]map[string]any, len(group))
	for i := range group {
		refs, problems := resolveRefs(x, s, group[i].parsed.Fields)
		if len(problems) > 0 {
			return nil, httpx.NewError(http.StatusConflict, "IMPORT_LINE_FAILED", fmt.Sprintf("%s row %d: %s Nothing was imported.", s.Title, group[i].parsed.Row, problems[0]))
		}
		bodies[i] = body(s, group[i].parsed.Fields, refs)
	}
	return bodies, nil
}

func checkStates(todo []pendingRecord, mayAllocate bool) error {
	for i := range todo {
		if !mayAllocate && todo[i].parsed.Fields["allocation_state"] != "" {
			return httpx.NewError(http.StatusForbidden, "IMPORT_NEEDS_ALLOCATE", "This file sets allocation states, so its approver must be allowed to allocate or reserve addresses.")
		}
	}
	return nil
}

// commitSheets creates every valid row, parents first. Rows are looked up again here
// because the inventory may have changed since the upload was checked.
func commitSheets(ctx context.Context, tx pgx.Tx, actor string, todo []pendingRecord, mayAllocate bool) error {
	if err := checkStates(todo, mayAllocate); err != nil {
		return err
	}
	x := newIndex(tx)
	var records, created, objects []string
	for i := range sheets {
		s := &sheets[i]
		var group []pendingRecord
		for _, p := range todo {
			if p.parsed.Type == s.Key {
				group = append(group, p)
			}
		}
		if len(group) == 0 {
			continue
		}
		bodies, err := sheetBodies(ctx, x, s, group)
		if err != nil {
			return err
		}
		ids, err := writeSheet(ctx, tx, actor, s, group, bodies)
		if err != nil {
			return err
		}
		if s.Key == "network_blocks" {
			if err := nestBlocks(ctx, tx, ids); err != nil {
				return err
			}
		}
		for j, id := range ids {
			records, created, objects = append(records, group[j].id), append(created, id), append(objects, s.Object)
		}
	}
	_, err := tx.Exec(ctx, `UPDATE import_records r SET status = 'committed', created_object_type = t.kind, created_object_id = t.made
		FROM unnest($1::uuid[], $2::uuid[], $3::text[]) AS t(id, made, kind) WHERE r.id = t.id`, records, created, objects)
	return err
}
