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
		id = r.x.ids["circuits"][circuitKey(provider, circuit)]
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
		return "", "", rowError(r.x.codeOf(id) + " already exists. Tick “Update existing links” to change it from this file.")
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
		v, ok := cells[strings.ToLower(c.Header)]
		if old, renamed := oldHeaders[c.Header]; !ok && renamed {
			v, ok = cells[old]
		}
		if ok {
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

// codeOf is a link's Link ID, from the lookup read at the start of the import.
func (x *resolver) codeOf(id string) string {
	for code, v := range x.ids["links"] {
		if v == id {
			return strings.ToUpper(code)
		}
	}
	return "This link"
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
			ExtraWANIPs: r.ExtraWANIPs,
			PublicIPs:   deref(r.PublicIPs), GatewayIP: deref(r.GatewayIP), DeviceIDs: r.DeviceIDs, DeviceText: r.DeviceText,
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
