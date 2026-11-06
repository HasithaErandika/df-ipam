package imports

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
)

// rowCheck is a row and what the checks found.
type rowCheck struct {
	row    *sheetRow
	status string
	errors []string
	refs   map[string]*entry
	keys   []string // the row's unique name, then its second name if it has one
}

func (r *rowCheck) fail(status, format string, args ...any) {
	r.status = status
	r.errors = append(r.errors, fmt.Sprintf(format, args...))
}

func (r *rowCheck) valid() bool { return r.status == "valid" }

// refName is the name a reference cell looks up, within its scope if it has one.
func refName(fields map[string]string, c *column) string {
	v := fields[c.Name]
	if v != "" && c.Within != "" {
		return fields[c.Within] + "/" + v
	}
	return v
}

func refLabel(kind string) string {
	if s := sheetByKey(kind); s != nil {
		return strings.ToLower(s.Title)
	}
	return "active users"
}

// resolveRefs finds every reference of a row, and says which can't be followed.
func resolveRefs(x *index, s *sheet, fields map[string]string) (map[string]*entry, []string) {
	refs := map[string]*entry{}
	var problems []string
	for i := range s.Columns {
		c := &s.Columns[i]
		name := refName(fields, c)
		if c.Ref == "" || name == "" {
			continue
		}
		switch found := x.find(c.Ref, name); len(found) {
		case 0:
			problems = append(problems, fmt.Sprintf("%s: %q isn't in %s, here or in the inventory.", c.Name, fields[c.Name], refLabel(c.Ref)))
		case 1:
			refs[c.Name] = found[0]
		default:
			problems = append(problems, fmt.Sprintf("%s: %q matches %d records. Write it with its parent, such as CODE/%s.", c.Name, fields[c.Name], len(found), fields[c.Name]))
		}
	}
	return refs, problems
}

func refNames(s *sheet, rows []*sheetRow) map[string][]string {
	out := map[string][]string{}
	for _, r := range rows {
		for i := range s.Columns {
			c := &s.Columns[i]
			if name := refName(r.fields, c); c.Ref != "" && name != "" {
				out[c.Ref] = append(out[c.Ref], nameOf(name))
			}
		}
	}
	return out
}

func loadRefs(ctx context.Context, x *index, s *sheet, rows []*sheetRow) error {
	for kind, names := range refNames(s, rows) {
		if err := x.load(ctx, kind, names); err != nil {
			return err
		}
	}
	return nil
}

func rowKeys(s *sheet, r *rowCheck) []string {
	prefix := ""
	if p := r.refs[s.Parent]; p != nil {
		prefix = p.names[0] + "/"
	}
	keys := []string{prefix + nameOf(r.row.fields[s.KeyCol])}
	if v := r.row.fields[s.AltCol]; s.AltCol != "" && v != "" {
		keys = append(keys, prefix+nameOf(v))
	}
	return keys
}

// markDuplicates finds rows already in the inventory or earlier in the file. They
// are skipped, never used to overwrite what is there.
func markDuplicates(ctx context.Context, x *index, s *sheet, rows []*rowCheck) error {
	var names []string
	for _, r := range rows {
		names = append(names, r.keys...)
	}
	if err := x.load(ctx, s.Key, names); err != nil {
		return err
	}
	seen := map[string]int{}
	for _, r := range rows {
		if !r.valid() {
			continue
		}
		for _, key := range r.keys {
			if first, dup := seen[key]; dup {
				r.fail("duplicate", "Same as row %d.", first)
			} else if slices.ContainsFunc(x.find(s.Key, key), func(e *entry) bool { return e.names[0] == r.keys[0] || key != r.keys[0] }) {
				r.fail("duplicate", "Already in the inventory. Existing records are never changed by an import.")
			}
			seen[key] = r.row.number
		}
	}
	return nil
}

func register(x *index, s *sheet, r *rowCheck) {
	bare := []string{nameOf(r.row.fields[s.KeyCol])}
	if v := r.row.fields[s.AltCol]; s.AltCol != "" && v != "" {
		bare = append(bare, nameOf(v))
	}
	var names []string
	if p := r.refs[s.Parent]; p != nil {
		for _, b := range bare {
			for _, n := range p.names {
				names = append(names, n+"/"+b)
			}
		}
	}
	x.add(s.Key, &entry{id: fmt.Sprintf("new:%d", r.row.line), names: append(names, bare...)})
}

func checkSheet(ctx context.Context, x *index, n *networks, s *sheet, rows []*sheetRow) ([]*rowCheck, error) {
	if err := loadRefs(ctx, x, s, rows); err != nil {
		return nil, err
	}
	var valid, out []*rowCheck
	for _, row := range rows {
		r := &rowCheck{row: row, status: "valid"}
		checkCells(r)
		var problems []string
		r.refs, problems = resolveRefs(x, s, row.fields)
		for _, p := range problems {
			r.fail("invalid", "%s", p)
		}
		out = append(out, r)
		if r.valid() {
			r.keys = rowKeys(s, r)
			valid = append(valid, r)
		}
	}
	if err := markDuplicates(ctx, x, s, valid); err != nil {
		return nil, err
	}
	if err := n.check(ctx, s, valid); err != nil {
		return nil, err
	}
	for _, r := range valid {
		if r.valid() {
			register(x, s, r)
		}
	}
	return out, nil
}

// checkTemplate checks every row, parents first, so a row can use one above it. A
// row that failed can't be used: anything pointing at it fails too and says so.
func checkTemplate(ctx context.Context, tx pgx.Tx, rows []*sheetRow) ([]*rowCheck, error) {
	x, n := newIndex(tx), newNetworks(tx)
	var out []*rowCheck
	for i := range sheets {
		s := &sheets[i]
		var group []*sheetRow
		for _, r := range rows {
			if r.sheet == s {
				group = append(group, r)
			}
		}
		if len(group) == 0 {
			continue
		}
		checked, err := checkSheet(ctx, x, n, s, group)
		if err != nil {
			return nil, err
		}
		out = append(out, checked...)
	}
	return out, nil
}
