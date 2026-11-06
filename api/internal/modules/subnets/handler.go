package subnets

import (
	"net/http"
	"strconv"
	"strings"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/netcalc"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc} }

func (h *Handler) Register(g *auth.Guard) {
	g.Handle(http.MethodGet, "/subnets", h.list, auth.InventoryView, auth.InventoryViewScoped)
	g.Handle(http.MethodPost, "/subnets", h.create, auth.InventoryCreate)
	g.Handle(http.MethodPost, "/subnets/calculate", h.calculate, auth.InventoryView)
	g.Handle(http.MethodPost, "/subnets/check-overlap", h.checkOverlap, auth.InventoryView)
	g.Handle(http.MethodPost, "/subnets/bulk", h.bulkCreate, auth.InventoryCreate)
	g.Handle(http.MethodPost, "/subnets/bulk-archive", h.bulkArchive, auth.InventoryEdit)
	g.Handle(http.MethodGet, "/subnets/:id", h.get, auth.InventoryView, auth.InventoryViewScoped)
	g.Handle(http.MethodPatch, "/subnets/:id", h.update, auth.InventoryEdit, auth.InventoryEditAssigned)
	g.Handle(http.MethodPost, "/subnets/:id/archive", h.archive, auth.InventoryEdit)
	g.Handle(http.MethodDelete, "/subnets/:id", h.remove, auth.InventoryDelete)
	g.Handle(http.MethodGet, "/subnets/:id/utilisation", h.utilisation, auth.InventoryView)
	g.Handle(http.MethodGet, "/subnets/:id/free-ranges", h.freeRanges, auth.InventoryView)
	g.Handle(http.MethodGet, "/subnets/:id/addresses", h.addresses, auth.InventoryView)
	g.Handle(http.MethodGet, "/network-blocks/:id/free-space", h.blockFreeSpace, auth.InventoryView)
	g.Handle(http.MethodPost, "/tools/split", h.split, auth.InventoryView)
	g.Handle(http.MethodPost, "/tools/summarise", h.summarise, auth.InventoryView)
	g.Handle(http.MethodGet, "/tools/locate", h.locate, auth.InventoryView)
}

func (h *Handler) list(c *gin.Context) {
	p := httpx.ParsePage(c)
	family, _ := strconv.Atoi(c.Query("family"))
	rd := c.Query("routingDomainId")
	if rd != "" && !httpx.IsUUID(rd) {
		httpx.Fail(c, httpx.Validation("routingDomainId", "Must be a valid ID."))
		return
	}
	for _, param := range []string{"siteId", "vlanId", "vdomId", "businessServiceId", "networkBlockId"} {
		if v := c.Query(param); v != "" && !httpx.IsUUID(v) {
			httpx.Fail(c, httpx.Validation(param, "Must be a valid ID."))
			return
		}
	}
	scope, err := auth.ServiceScope(c.Request.Context(), h.svc.pool, auth.Current(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	rows, total, err := h.svc.List(c.Request.Context(), Filter{
		Services: scope,
		SiteID:   c.Query("siteId"), VLANID: c.Query("vlanId"), VDOMID: c.Query("vdomId"),
		BusinessServiceID: c.Query("businessServiceId"), NetworkBlockID: c.Query("networkBlockId"),
		Query: strings.TrimSpace(c.Query("q")), RoutingDomainID: rd, Environment: c.Query("environment"),
		Family: family, IncludeArchived: c.Query("includeArchived") == "true", Sort: c.Query("sort"),
	}, p)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if scope != nil {
		for i := range rows {
			rows[i].hideInfrastructure()
		}
	}
	httpx.List(c, rows, p, total)
}

func (h *Handler) get(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	row, err := h.svc.Get(c.Request.Context(), id)
	if err == nil {
		err = h.limit(c, &row)
	}
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, row)
}

func (h *Handler) create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	if !httpx.IsUUID(req.RoutingDomainID) {
		httpx.Fail(c, httpx.Validation("routingDomainId", "Choose a routing domain."))
		return
	}
	row, err := h.svc.Create(c.Request.Context(), auth.Current(c).UserID, req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusCreated, row)
}

func (h *Handler) update(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	version, err := strconv.ParseInt(strings.Trim(c.GetHeader("If-Match"), `"`), 10, 32)
	if err != nil {
		httpx.Fail(c, httpx.NewError(http.StatusPreconditionFailed, "STALE_VERSION", "Send the record version in If-Match."))
		return
	}
	var req UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	row, err := h.write(c, id, int32(version), req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, row)
}

func (h *Handler) archive(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	if err := h.svc.Archive(c.Request.Context(), auth.Current(c).UserID, id); err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) remove(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), auth.Current(c).UserID, id); err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) calculate(c *gin.Context) {
	var req struct {
		CIDR string `json:"cidr"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	p, err := ParseCIDR(req.CIDR)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, netcalc.Calculate(p))
}

func (h *Handler) checkOverlap(c *gin.Context) {
	var req OverlapRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	if !httpx.IsUUID(req.RoutingDomainID) {
		httpx.Fail(c, httpx.Validation("routingDomainId", "Choose a routing domain."))
		return
	}
	conflicts, err := h.svc.CheckOverlap(c.Request.Context(), req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, gin.H{"overlaps": len(conflicts) > 0, "conflicts": conflicts})
}
