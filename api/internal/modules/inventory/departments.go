package inventory

import (
	"context"
	"errors"
	"strings"

	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/jackc/pgx/v5"
)

const pickDepartment = "Choose a department from the list. Administrators add departments in Network setup."

// checkDepartment matches a department to the list without regard to case and
// sets its stored spelling. Blank is allowed.
func checkDepartment(ctx context.Context, conn db.Conn, name *string) error {
	if strings.TrimSpace(*name) == "" {
		*name = ""
		return nil
	}
	err := conn.QueryRow(ctx, "SELECT name FROM departments WHERE lower(name) = lower($1)", strings.TrimSpace(*name)).Scan(name)
	if errors.Is(err, pgx.ErrNoRows) {
		return httpx.Validation("department", pickDepartment)
	}
	return err
}

// departmentsByName maps each department, lower-cased, to its stored spelling, for imports.
func departmentsByName(ctx context.Context, conn db.Conn) (map[string]string, error) {
	rows, err := conn.Query(ctx, "SELECT lower(name), name FROM departments")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var key, name string
		if err := rows.Scan(&key, &name); err != nil {
			return nil, err
		}
		out[key] = name
	}
	return out, rows.Err()
}

// importDepartment checks one import cell against the list, returning its
// stored spelling, or a problem for the report.
func importDepartment(cell string, list map[string]string) (string, string) {
	cell = strings.TrimSpace(cell)
	if cell == "" {
		return "", ""
	}
	if name, ok := list[strings.ToLower(cell)]; ok {
		return name, ""
	}
	return "", "Unknown department " + cell + ". Add it in Network setup first."
}
