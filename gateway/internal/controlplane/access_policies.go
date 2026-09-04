package controlplane

import (
	"net/http"

	"github.com/atsokha/mcplake/router"
	"github.com/gin-gonic/gin"
)

type claimRuleDTO struct {
	Path    string `json:"path" binding:"required"`
	Pattern string `json:"pattern" binding:"required"`
}

type grantDTO struct {
	MCP   string   `json:"mcp" binding:"required"`
	Tools []string `json:"tools"`
}

type accessPolicyDTO struct {
	Name   string         `json:"name"`
	Match  []claimRuleDTO `json:"match"`
	Grants []grantDTO     `json:"grants"`
}

func accessPolicyDTOFrom(p router.AccessPolicy) accessPolicyDTO {
	match := make([]claimRuleDTO, len(p.Match.Rules))
	for i, r := range p.Match.Rules {
		match[i] = claimRuleDTO{Path: r.Path, Pattern: r.Pattern}
	}
	grants := make([]grantDTO, len(p.Grants))
	for i, g := range p.Grants {
		grants[i] = grantDTO{MCP: g.MCP, Tools: g.Tools}
	}
	return accessPolicyDTO{Name: p.Name, Match: match, Grants: grants}
}

type accessPolicyRequest struct {
	Name   string         `json:"name"`
	Match  []claimRuleDTO `json:"match"`
	Grants []grantDTO     `json:"grants"`
}

func (r accessPolicyRequest) toAccessPolicy(name string) router.AccessPolicy {
	rules := make([]router.ClaimRule, len(r.Match))
	for i, m := range r.Match {
		rules[i] = router.ClaimRule{Path: m.Path, Pattern: m.Pattern}
	}
	grants := make([]router.Grant, len(r.Grants))
	for i, g := range r.Grants {
		grants[i] = router.Grant{MCP: g.MCP, Tools: g.Tools}
	}
	return router.AccessPolicy{Name: name, Match: router.ClaimMatcher{Rules: rules}, Grants: grants}
}

type accessPolicyHandlers struct {
	repo     accessPolicyRepository
	reloader *PolicyReloader
}

// RegisterAccessPolicyRoutes registers POST/GET/PUT/DELETE
// /admin/access-policies[/:name] onto admin. Every write persists via repo
// and then calls reloader.Refresh so the change is immediately visible to
// the data plane's PolicyStore.Authorize/FieldsToRemove.
func RegisterAccessPolicyRoutes(admin *gin.RouterGroup, repo accessPolicyRepository, reloader *PolicyReloader) {
	h := &accessPolicyHandlers{repo: repo, reloader: reloader}
	admin.POST("/access-policies", h.create)
	admin.GET("/access-policies", h.list)
	admin.GET("/access-policies/:name", h.get)
	admin.PUT("/access-policies/:name", h.update)
	admin.DELETE("/access-policies/:name", h.delete)
}

func (h *accessPolicyHandlers) create(c *gin.Context) {
	var req accessPolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid_request", Message: err.Error()})
		return
	}
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid_request", Message: "name is required"})
		return
	}

	h.upsertAndRespond(c, req.toAccessPolicy(req.Name), http.StatusCreated)
}

func (h *accessPolicyHandlers) update(c *gin.Context) {
	name := c.Param("name")
	var req accessPolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid_request", Message: err.Error()})
		return
	}

	h.upsertAndRespond(c, req.toAccessPolicy(name), http.StatusOK)
}

func (h *accessPolicyHandlers) upsertAndRespond(c *gin.Context, policy router.AccessPolicy, successStatus int) {
	ctx := c.Request.Context()
	if err := h.repo.Upsert(ctx, policy); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid_policy", Message: err.Error()})
		return
	}

	if err := h.reloader.Refresh(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "internal_error", Message: err.Error()})
		return
	}

	c.JSON(successStatus, accessPolicyDTOFrom(policy))
}

func (h *accessPolicyHandlers) list(c *gin.Context) {
	policies, err := h.repo.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "internal_error", Message: err.Error()})
		return
	}
	dtos := make([]accessPolicyDTO, 0, len(policies))
	for _, p := range policies {
		dtos = append(dtos, accessPolicyDTOFrom(p))
	}
	c.JSON(http.StatusOK, dtos)
}

func (h *accessPolicyHandlers) get(c *gin.Context) {
	name := c.Param("name")
	policy, ok, err := h.repo.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "internal_error", Message: err.Error()})
		return
	}
	if !ok {
		c.JSON(http.StatusNotFound, errorResponse{Error: "not_found", Message: "access policy not found"})
		return
	}
	c.JSON(http.StatusOK, accessPolicyDTOFrom(policy))
}

func (h *accessPolicyHandlers) delete(c *gin.Context) {
	name := c.Param("name")
	ctx := c.Request.Context()

	_, ok, err := h.repo.Get(ctx, name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "internal_error", Message: err.Error()})
		return
	}
	if !ok {
		c.JSON(http.StatusNotFound, errorResponse{Error: "not_found", Message: "access policy not found"})
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
