package controlplane

import (
	"net/http"

	"github.com/atsokha/mcplake/router"
	"github.com/gin-gonic/gin"
)

type filterPolicyDTO struct {
	Name       string         `json:"name"`
	Match      []claimRuleDTO `json:"match"`
	MCP        string         `json:"mcp"`
	Tool       string         `json:"tool"`
	DropFields []string       `json:"drop_fields"`
	Enabled    bool           `json:"enabled"`
}

func filterPolicyDTOFrom(p router.FilterPolicy) filterPolicyDTO {
	match := make([]claimRuleDTO, len(p.Match.Rules))
	for i, r := range p.Match.Rules {
		match[i] = claimRuleDTO{Path: r.Path, Pattern: r.Pattern}
	}
	return filterPolicyDTO{
		Name:       p.Name,
		Match:      match,
		MCP:        p.MCP,
		Tool:       p.Tool,
		DropFields: p.DropFields,
		Enabled:    p.Enabled,
	}
}

type filterPolicyRequest struct {
	Name       string         `json:"name"`
	Match      []claimRuleDTO `json:"match"`
	MCP        string         `json:"mcp" binding:"required"`
	Tool       string         `json:"tool" binding:"required"`
	DropFields []string       `json:"drop_fields"`
	// Enabled is optional; omitting it means enabled (see enabledOrTrue).
	Enabled *bool `json:"enabled"`
}

func (r filterPolicyRequest) toFilterPolicy(name string) router.FilterPolicy {
	rules := make([]router.ClaimRule, len(r.Match))
	for i, m := range r.Match {
		rules[i] = router.ClaimRule{Path: m.Path, Pattern: m.Pattern}
	}
	return router.FilterPolicy{
		Name:       name,
		Match:      router.ClaimMatcher{Rules: rules},
		MCP:        r.MCP,
		Tool:       r.Tool,
		DropFields: r.DropFields,
		Enabled:    enabledOrTrue(r.Enabled),
	}
}

type filterPolicyHandlers struct {
	repo     filterPolicyRepository
	reloader *PolicyReloader
}

// RegisterFilterPolicyRoutes registers POST/GET/PUT/DELETE
// /admin/filter-policies[/:name] onto admin, mirroring
// RegisterAccessPolicyRoutes' shape (#48). Every write persists via repo
// and then calls reloader.Refresh so the change is immediately visible to
// the data plane's PolicyStore.FieldsToRemove.
func RegisterFilterPolicyRoutes(admin *gin.RouterGroup, repo filterPolicyRepository, reloader *PolicyReloader) {
	h := &filterPolicyHandlers{repo: repo, reloader: reloader}
	admin.POST("/filter-policies", h.create)
	admin.GET("/filter-policies", h.list)
	admin.GET("/filter-policies/:name", h.get)
	admin.PUT("/filter-policies/:name", h.update)
	admin.DELETE("/filter-policies/:name", h.delete)
}

// @Summary      Create a filter policy
// @Tags         filter-policies
// @Accept       json
// @Produce      json
// @Param        request  body      filterPolicyRequest  true  "Filter policy"
// @Success      201      {object}  filterPolicyDTO
// @Failure      400      {object}  errorResponse
// @Router       /filter-policies [post]
func (h *filterPolicyHandlers) create(c *gin.Context) {
	var req filterPolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid_request", Message: err.Error()})
		return
	}
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid_request", Message: "name is required"})
		return
	}

	h.upsertAndRespond(c, req.toFilterPolicy(req.Name), http.StatusCreated)
}

// @Summary      Replace a filter policy
// @Description  Creates it if absent.
// @Tags         filter-policies
// @Accept       json
// @Produce      json
// @Param        name     path      string                true  "Policy name"
// @Param        request  body      filterPolicyRequest  true  "Filter policy"
// @Success      200      {object}  filterPolicyDTO
// @Failure      400      {object}  errorResponse
// @Router       /filter-policies/{name} [put]
func (h *filterPolicyHandlers) update(c *gin.Context) {
	name := c.Param("name")
	var req filterPolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid_request", Message: err.Error()})
		return
	}

	h.upsertAndRespond(c, req.toFilterPolicy(name), http.StatusOK)
}

func (h *filterPolicyHandlers) upsertAndRespond(c *gin.Context, policy router.FilterPolicy, successStatus int) {
	ctx := c.Request.Context()
	if err := h.repo.Upsert(ctx, policy); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid_policy", Message: err.Error()})
		return
	}

	if err := h.reloader.Refresh(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "internal_error", Message: err.Error()})
		return
	}

	c.JSON(successStatus, filterPolicyDTOFrom(policy))
}

// @Summary      List filter policies
// @Tags         filter-policies
// @Produce      json
// @Success      200  {array}  filterPolicyDTO
// @Router       /filter-policies [get]
func (h *filterPolicyHandlers) list(c *gin.Context) {
	policies, err := h.repo.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "internal_error", Message: err.Error()})
		return
	}
	dtos := make([]filterPolicyDTO, 0, len(policies))
	for _, p := range policies {
		dtos = append(dtos, filterPolicyDTOFrom(p))
	}
	c.JSON(http.StatusOK, dtos)
}

// @Summary      Get a filter policy
// @Tags         filter-policies
// @Produce      json
// @Param        name  path      string  true  "Policy name"
// @Success      200   {object}  filterPolicyDTO
// @Failure      404   {object}  errorResponse
// @Router       /filter-policies/{name} [get]
func (h *filterPolicyHandlers) get(c *gin.Context) {
	name := c.Param("name")
	policy, ok, err := h.repo.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "internal_error", Message: err.Error()})
		return
	}
	if !ok {
		c.JSON(http.StatusNotFound, errorResponse{Error: "not_found", Message: "filter policy not found"})
		return
	}
	c.JSON(http.StatusOK, filterPolicyDTOFrom(policy))
}

// @Summary      Delete a filter policy
// @Tags         filter-policies
// @Produce      json
// @Param        name  path  string  true  "Policy name"
// @Success      204
// @Failure      404  {object}  errorResponse
// @Router       /filter-policies/{name} [delete]
func (h *filterPolicyHandlers) delete(c *gin.Context) {
	name := c.Param("name")
	ctx := c.Request.Context()

	_, ok, err := h.repo.Get(ctx, name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "internal_error", Message: err.Error()})
		return
	}
	if !ok {
		c.JSON(http.StatusNotFound, errorResponse{Error: "not_found", Message: "filter policy not found"})
		return
	}

	if err := h.repo.Delete(ctx, name); err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "internal_error", Message: err.Error()})
		return
	}

	if err := h.reloader.Refresh(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "internal_error", Message: err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}
