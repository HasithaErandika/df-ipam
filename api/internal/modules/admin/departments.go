package admin

import (
	"encoding/json"
	"net/http"

	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/validate"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type departmentDTO struct {
	ID          string `db:"id" json:"id"`
	Name        string `db:"name" json:"name"`
	Description string `db:"description" json:"description"`
	Devices     int64  `db:"devices" json:"devices"`
	Addresses   int64  `db:"addresses" json:"addresses"`
}

// Devices and register entries hold the department by name, matched without case.
const departmentUse = `(SELECT count(*) FROM devices x WHERE lower(x.department) = lower(dp.name)) AS devices,
	(SELECT count(*) FROM ip_addresses x WHERE lower(x.department) = lower(dp.name)) AS addresses`

func (h *Handler) departments(c *gin.Context) {
	out, err := db.All[departmentDTO](c, h.conn, `SELECT dp.id::text AS id, dp.name, dp.description, `+departmentUse+`
		FROM departments dp ORDER BY lower(dp.name)`)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

type departmentInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (in *departmentInput) validate() error {
	var v validate.V
	v.Text("name", &in.Name, 120, true, "Name")
	v.Text("description", &in.Description, 300, false, "Description")
	return v.Err()
}

func (h *Handler) createDepartment(c *gin.Context) { h.saveDepartment(c, "") }

func (h *Handler) updateDepartment(c *gin.Context) {
	if id, ok := httpx.PathID(c); ok {
		h.saveDepartment(c, id)
	}
}

// saveDepartment adds or edits one. A rename carries over to the devices and
// register entries that use the old name, in the same transaction.
func (h *Handler) saveDepartment(c *gin.Context, id string) {
	var in departmentInput
	if !httpx.Bind(c, &in) {
		return
	}
	if err := in.validate(); err != nil {
		httpx.Fail(c, err)
		return
	}
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		if id == "" {
			if err := tx.QueryRow(c, `INSERT INTO departments (name, description) VALUES ($1, $2) RETURNING id::text`, in.Name, in.Description).Scan(&id); err != nil {
				return err
			}
			return record(c, tx, "create", "department", id, nil, in)
		}
		var old []byte
		var oldName string
		if err := tx.QueryRow(c, "SELECT to_jsonb(dp), dp.name FROM departments dp WHERE id = $1::uuid FOR UPDATE", id).Scan(&old, &oldName); err != nil {
			return err
		}
		if _, err := tx.Exec(c, "UPDATE departments SET name = $1, description = $2 WHERE id = $3::uuid", in.Name, in.Description, id); err != nil {
			return err
		}
		if oldName != in.Name {
			if err := renameDepartment(c, tx, oldName, in.Name); err != nil {
				return err
			}
		}
		return record(c, tx, "update", "department", id, json.RawMessage(old), in)
	})
	if err != nil {
		httpx.Fail(c, db.Friendly(err, "A department with this name already exists."))
		return
	}
	httpx.OK(c, http.StatusOK, gin.H{"id": id})
}

func renameDepartment(c *gin.Context, tx pgx.Tx, from, to string) error {
	if _, err := tx.Exec(c, "UPDATE devices SET department = $2 WHERE lower(department) = lower($1)", from, to); err != nil {
		return err
	}
	_, err := tx.Exec(c, "UPDATE ip_addresses SET department = $2 WHERE lower(department) = lower($1)", from, to)
	return err
}

// deleteDepartment removes one nobody uses, so no entry is left naming a
// department that no longer exists.
func (h *Handler) deleteDepartment(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		var old []byte
		var inUse bool
		if err := tx.QueryRow(c, `SELECT to_jsonb(dp), EXISTS (SELECT 1 FROM devices x WHERE lower(x.department) = lower(dp.name))
			OR EXISTS (SELECT 1 FROM ip_addresses x WHERE lower(x.department) = lower(dp.name))
			FROM departments dp WHERE id = $1::uuid FOR UPDATE`, id).Scan(&old, &inUse); err != nil {
			return err
		}
		if inUse {
			return httpx.NewError(http.StatusConflict, "DEPARTMENT_IN_USE", "Devices or register entries still use this department. Move them first.")
		}
		if _, err := tx.Exec(c, "DELETE FROM departments WHERE id = $1::uuid", id); err != nil {
			return err
		}
		return record(c, tx, "delete", "department", id, json.RawMessage(old), nil)
	})
	if err != nil {
		httpx.Fail(c, db.Friendly(err))
		return
	}
	c.Status(http.StatusNoContent)
}
