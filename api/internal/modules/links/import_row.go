package links

import (
	"strings"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"

	"github.com/jackc/pgx/v5"
)

// target finds the link a row updates, by Link ID or by provider and circuit
// ID, and checks the person may add or update it.
func (r *importRun) target(tx pgx.Tx, cells map[string]string) (string, string, error) {
	id := ""
	if code := cells["link id"]; code != "" {
		if id = r.x.ids["links"][strings.ToLower(code)]; id == "" {
			return "", "", rowError("Link ID " + code + " doesn't exist. Leave Link ID empty to add a new link.")
		}
	} else if provider, circuit := cells["service provider"], cells["circuit id"]; provider != "" && circuit != "" {
		_ = tx.QueryRow(r.c, `SELECT l.id::text FROM network_links l JOIN network_link_providers p ON p.id = l.provider_id
			WHERE lower(p.name) = lower($1) AND lower(l.circuit_id) = lower($2) AND l.archived_at IS NULL`, provider, circuit).Scan(&id)
	}
	if id == "" {
		if !can(r.c, auth.LinksCreate) {
			return "", "", rowError("You may update links but not add them.")
		}
		return "", "add", nil
	}
	if !can(r.c, auth.LinksEdit) {
		return "", "", rowError("This link already exists, and you may add links but not change them.")
	}
	if !r.update {
		var code string
		_ = tx.QueryRow(r.c, `SELECT code FROM network_links WHERE id = $1::uuid`, id).Scan(&code)
		return "", "", rowError(code + " already exists. Tick “Update existing links” to change it from this file.")
	}
	return id, "update", nil
}

// input starts from the link as stored (or a new one) and applies the columns
// the file has. A column the file leaves out keeps its value.
func (r *importRun) input(tx pgx.Tx, id string, cells map[string]string) (*linkInput, error) {
	in := &linkInput{Status: "active", commercial: commercial{Currency: "LKR"}}
	if id != "" {
		var w db.Where
		w.Add("l.id = ?::uuid", id)
		rows, err := ioRows(r.c, tx, w, r.s)
		if err != nil || len(rows) == 0 {
			return nil, rowError("This link couldn't be read.")
		}
		in = inputFrom(rows[0], r.x)
	}
	r.x.issues = nil
	for _, c := range ioColumns {
		if v, ok := cells[strings.ToLower(c.Header)]; ok {
			c.set(in, v, r.x)
		}
	}
	if in.Status == "" {
		in.Status = "active"
	}
	if len(r.x.issues) > 0 {
		return nil, rowError(strings.Join(r.x.issues, " "))
	}
	return in, nil
}

func deref(p *string) string { return str(p) }

func intOf(p *int32) *int {
	if p == nil {
		return nil
	}
	i := int(*p)
	return &i
}

// inputFrom turns a stored link back into the form, for an update that
// changes some columns only.
func inputFrom(r *ioRow, x *resolver) *linkInput {
	in := &linkInput{
		Name: r.Name, Type: r.Type, ProviderID: deref(r.ProviderID), CircuitID: r.CircuitID, SourceSite: r.SourceSite,
		Destination: r.Destination, Purpose: r.Purpose, Environment: r.Environment, Role: r.Role, Criticality: r.Criticality,
		OfficerID: deref(r.OfficerID), Status: r.Status, Remarks: r.Remarks,
		technical: technical{DownMbps: r.DownMbps, UpMbps: r.UpMbps, Medium: r.Medium, WANIP: deref(r.WANIP), WANSegmentID: deref(r.WANSegmentID),
			PublicIPs: deref(r.PublicIPs), GatewayIP: deref(r.GatewayIP), DeviceID: deref(r.DeviceID), DeviceText: r.DeviceText,
			Interface: r.Interface, VLAN: intOf(r.VLAN), VRF: r.VRF, Routing: r.Routing},
		vpn: vpn{VPNTunnel: r.VPNTunnel, VPNType: r.VPNType, LocalPeer: r.LocalPeer, RemotePeer: r.RemotePeer, RemoteSubnets: r.RemoteSubnets,
			RemoteOrg: r.RemoteOrg, RemoteContact: r.RemoteContact, LastTestDate: deref(r.LastTestDate), VaultRef: r.VaultRef},
		commercial: commercial{NoSeparateCost: r.NoSeparateCost, BillingAccount: r.BillingAccount, MonthlyRental: r.MonthlyRental,
			Currency: r.Currency, InstallCost: r.InstallCost, ContractRef: r.ContractRef, PORef: r.PORef, ActivationDate: deref(r.ActivationDate),
			ContractStart: deref(r.ContractStart), ContractEnd: deref(r.ContractEnd), RenewalNoticeDays: intOf(r.RenewalNoticeDays), CostCentre: r.CostCentre},
		support: support{SupportContact: r.SupportContact, SupportEmail: r.SupportEmail, SupportHotline: r.SupportHotline,
			AccountManager: r.AccountManager, SLAPct: r.SLAPct, ResolutionHours: r.ResolutionHours},
	}
	in.LocalSegmentIDs = x.findAll("segments", strings.Join(r.LocalSubnets, ";"))
	in.Relations = x.relations(strings.Join(r.Related, ";"))
	return in
}

// rowError is a message for the import report, written for people.
type rowError string

func (e rowError) Error() string { return string(e) }
