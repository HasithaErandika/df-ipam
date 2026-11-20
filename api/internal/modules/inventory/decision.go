package inventory

import (
	"net/http"

	"dfipam/api/internal/ipam"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/platforms"
	"dfipam/api/internal/validate"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// decisionInput approves or rejects a device. Approval may also name it and add
// the address it was seen on to the register, in the same transaction.
type decisionInput struct {
	Status     string  `json:"status"`
	Name       string  `json:"name"`
	Owner      *string `json:"owner"`      // left out: kept; given, even blank: set
	Department *string `json:"department"` // the same
	Type       string  `json:"deviceType"`
	Platform   string  `json:"platform"`
	Register   *struct {
		SegmentID string `json:"segmentId"`
		IP        string `json:"ip"`
		IPType    string `json:"ipType"`
	} `json:"register"`
}

func (in *decisionInput) validate() error {
	var v validate.V
	v.OneOf("status", in.Status, []string{"approved", "rejected"}, "decision")
	v.Text("name", &in.Name, 120, false, "Name")
	if in.Owner != nil {
		v.Text("owner", in.Owner, 120, false, "Owner")
	}
	if in.Department != nil {
		v.Text("department", in.Department, 120, false, "Department")
	}
	if in.Type != "" {
		v.OneOf("deviceType", in.Type, deviceTypes, "device type")
	}
	if !platforms.ValidPlatform(in.Platform) {
		v.Add("platform", "Choose a listed platform.")
	}
	if r := in.Register; r != nil {
		if in.Status != "approved" {
			v.Add("register", "Only an approved device can be registered.")
		}
		v.UUID("register.segmentId", r.SegmentID, true, "a segment")
		v.IPv4("register.ip", &r.IP, true)
		if r.IPType == "" {
			r.IPType = "other"
		}
		v.OneOf("register.ipType", r.IPType, ipTypes, "IP type")
	}
	return v.Err()
}

func (h *Handler) decideDevice(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var in decisionInput
	if !httpx.Bind(c, &in) {
		return
	}
	if err := in.validate(); err != nil {
		httpx.Fail(c, err)
		return
	}
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		old, err := lockOld(c, tx, "devices", id)
		if err != nil {
			return err
		}
		if in.Department != nil {
			if err := checkDepartment(c, tx, in.Department); err != nil {
				return err
			}
		}
		var mac, owner, department string
		if err := tx.QueryRow(c, `UPDATE devices SET status = $2, decided_by = $3::uuid, decided_at = now(),
			name = COALESCE(NULLIF($4, ''), name), owner = COALESCE($5, owner),
			department = COALESCE($6, department), device_type = COALESCE(NULLIF($7, ''), device_type),
			platform = COALESCE(NULLIF($8, ''), platform)
			WHERE id = $1::uuid RETURNING mac::text, owner, department`, id, in.Status, actor(c), in.Name, in.Owner, in.Department, in.Type, in.Platform).Scan(&mac, &owner, &department); err != nil {
			return err
		}
		if err := record(c, tx, in.Status, "device", id, old, in); err != nil {
			return err
		}
		if in.Register == nil {
			return nil
		}
		if err := refuseReserved(c, tx, in.Register.SegmentID, in.Register.IP); err != nil {
			return err
		}
		if err := ipam.CheckRegister(c, tx, in.Register.SegmentID, in.Register.IP, in.Register.IPType); err != nil {
			return err
		}
		var ipID string
		if err := tx.QueryRow(c, `INSERT INTO ip_addresses (segment_id, ip, status, ip_type, device_id, assigned_name, expected_mac, owner, department, platform, created_by, updated_by)
			VALUES ($1::uuid, $2::inet, 'assigned', $3, $4::uuid, $5, $6::macaddr, $7, $8, $10, $9::uuid, $9::uuid) RETURNING id::text`,
			in.Register.SegmentID, in.Register.IP, in.Register.IPType, id, in.Name, mac, owner, department, actor(c), in.Platform).Scan(&ipID); err != nil {
			return err
		}
		return record(c, tx, "create", "ip_address", ipID, nil, in.Register)
	})
	if err != nil {
		httpx.Fail(c, db.Friendly(err, "That IP is already in the register."))
		return
	}
	h.recheck(c)
	c.Status(http.StatusNoContent)
}
