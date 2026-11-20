// Package firewall reviews what the connected FortiGate knows (networks,
// devices and servers) against DF-IPAM's records. It only reads: people tick
// what to add, and the browser saves each item through the normal routes, so
// every check, permission and audit record applies.
package firewall

import (
	"context"
	"net/http"
	"time"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/integrations/fortigate"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
)

// Connector gives a client for the FortiGate set up in System > FortiGate.
type Connector interface {
	Connect(ctx context.Context) (*fortigate.Client, error)
}

type Handler struct {
	conn db.Conn
	fw   Connector
}

func New(conn db.Conn, fw Connector) *Handler { return &Handler{conn: conn, fw: fw} }

func (h *Handler) Register(g *auth.Guard) {
	g.Handle(http.MethodGet, "/firewall/inventory", h.review, auth.InventoryEdit, auth.NetworkManage)
}

type change struct {
	Field string `json:"field"`
	Old   any    `json:"old"`
	New   any    `json:"new"`
}

type inventory struct {
	Firewall string          `json:"firewall"`
	At       time.Time       `json:"at"`
	Networks []netProposal   `json:"networks"`
	Devices  []devProposal   `json:"devices"`
	Servers  []hostProposal  `json:"servers"`
	Problems []string        `json:"problems"`
	Segments []segmentRecord `json:"-"`
}

func (h *Handler) review(c *gin.Context) {
	if h.fw == nil {
		httpx.Fail(c, httpx.NewError(http.StatusConflict, "FIREWALL_NOT_CONNECTED", "Connect a FortiGate in System > FortiGate first: its address and a read-only API token."))
		return
	}
	client, err := h.fw.Connect(c)
	if err != nil {
		httpx.Fail(c, httpx.NewError(http.StatusConflict, "FIREWALL_NOT_CONNECTED", "The FortiGate isn't connected: "+err.Error()+"."))
		return
	}
	name, err := client.Status(c)
	if err != nil {
		httpx.Fail(c, httpx.NewError(http.StatusBadGateway, "FIREWALL_UNREACHABLE", "The FortiGate didn't answer: "+err.Error()+"."))
		return
	}
	segs, err := db.All[segmentRecord](c, h.conn, `SELECT id::text AS id, name, kind, network::text AS network, COALESCE(host(gateway), '') AS gateway,
		vlan, COALESCE(host(dhcp_start), '') AS dhcp_start, COALESCE(host(dhcp_end), '') AS dhcp_end FROM segments ORDER BY network`)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out := inventory{Firewall: name, At: time.Now(), Networks: []netProposal{}, Devices: []devProposal{}, Servers: []hostProposal{}, Problems: []string{}, Segments: segs}
	dhcp, err := client.DHCPServers(c)
	if err != nil {
		out.Problems = append(out.Problems, "DHCP servers: "+err.Error())
	}
	if err := h.networks(c, client, &out, dhcp); err != nil {
		out.Problems = append(out.Problems, "Interfaces: "+err.Error())
	}
	if err := h.devices(c, client, &out); err != nil {
		httpx.Fail(c, err)
		return
	}
	addrs, err := client.Addresses(c)
	if err != nil {
		out.Problems = append(out.Problems, "Address objects: "+err.Error())
	}
	h.vpnPools(c, client, &out, addrs)
	if err := h.servers(c, client, &out, addrs, dhcp); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}
