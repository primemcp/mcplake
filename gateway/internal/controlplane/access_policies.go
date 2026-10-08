package controlplane

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/primemcp/mcplake/internal/controlplane/adminservice"
	"github.com/primemcp/mcplake/router"
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
	Name    string         `json:"name"`
	Match   []claimRuleDTO `json:"match"`
	Grants  []grantDTO     `json:"grants"`
	Enabled bool           `json:"enabled"`
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
	return accessPolicyDTO{Name: p.Name, Match: match, Grants: grants, Enabled: p.Enabled}
}

type accessPolicyRequest struct {
	Name   string         `json:"name"`
	Match  []claimRuleDTO `json:"match"`
	Grants []grantDTO     `json:"grants"`
	// Enabled is optional; omitting it means enabled (see enabledOrTrue).
	Enabled *bool `json:"enabled"`
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
	return router.AccessPolicy{
		Name:    name,
		Match:   router.ClaimMatcher{Rules: rules},
		Grants:  grants,
		Enabled: enabledOrTrue(r.Enabled),
	}
}

type accessPolicyHandlers struct {
	svc *adminservice.AccessPolicyService
}

// RegisterAccessPolicyRoutes registers POST/GET/PUT/DELETE
// /admin/access-policies[/:name] onto admin, backed by svc (the
// transport-agnostic access-policy application layer). Every write persists
// and then refreshes the live PolicyStore, so the change is immediately
// visible to the data plane's Authorize/FieldsToRemove calls.
func RegisterAccessPolicyRoutes(admin *gin.RouterGroup, svc *adminservice.AccessPolicyService) {
	h := &accessPolicyHandlers{svc: svc}
	admin.POST("/access-policies", h.create)
	admin.GET("/access-policies", h.list)
	admin.GET("/access-policies/:name", h.get)
	admin.PUT("/access-policies/:name", h.update)
	admin.DELETE("/access-policies/:name", h.delete)
}

// @Summary      Create an access policy
// @Tags         access-policies
// @Accept       json
// @Produce      json
// @Param        request  body      accessPolicyRequest  true  "Access policy"
// @Success      201      {object}  accessPolicyDTO
// @Failure      400      {object}  errorResponse
// @Router       /access-policies [post]
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

// @Summary      Replace an access policy
// @Description  Creates it if absent.
// @Tags         access-policies
// @Accept       json
// @Produce      json
// @Param        name     path      string                true  "Policy name"
// @Param        request  body      accessPolicyRequest  true  "Access policy"
// @Success      200      {object}  accessPolicyDTO
// @Failure      400      {object}  errorResponse
// @Router       /access-policies/{name} [put]
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
	if err := h.svc.Upsert(c.Request.Context(), policy); err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(successStatus, accessPolicyDTOFrom(policy))
}

// @Summary      List access policies
// @Tags         access-policies
// @Produce      json
// @Success      200  {array}  accessPolicyDTO
// @Router       /access-policies [get]
func (h *accessPolicyHandlers) list(c *gin.Context) {
	policies, err := h.svc.List(c.Request.Context())
	if err != nil {
		respondServiceError(c, err)
		return
	}
	dtos := make([]accessPolicyDTO, 0, len(policies))
	for _, p := range policies {
		dtos = append(dtos, accessPolicyDTOFrom(p))
	}
	c.JSON(http.StatusOK, dtos)
}

// @Summary      Get an access policy
// @Tags         access-policies
// @Produce      json
// @Param        name  path      string  true  "Policy name"
// @Success      200   {object}  accessPolicyDTO
// @Failure      404   {object}  errorResponse
// @Router       /access-policies/{name} [get]
func (h *accessPolicyHandlers) get(c *gin.Context) {
	policy, err := h.svc.Get(c.Request.Context(), c.Param("name"))
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, accessPolicyDTOFrom(policy))
}

// @Summary      Delete an access policy
// @Tags         access-policies
// @Produce      json
// @Param        name  path  string  true  "Policy name"
// @Success      204
// @Failure      404  {object}  errorResponse
// @Router       /access-policies/{name} [delete]
func (h *accessPolicyHandlers) delete(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), c.Param("name")); err != nil {
		respondServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
