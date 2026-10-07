package controlplane

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/primemcp/mcplake/cache"
	"github.com/primemcp/mcplake/internal/controlplane/adminservice"
)

type mcpHandlers struct {
	svc *adminservice.MCPService
}

// RegisterMCPRoutes registers POST/GET/PATCH/DELETE /admin/mcps[/:name] onto
// admin, backed by svc (the transport-agnostic MCP application layer). Every
// successful write goes through the live Registry first — so a caller of the
// data plane sees the effect immediately — then the durable store.
func RegisterMCPRoutes(admin *gin.RouterGroup, svc *adminservice.MCPService) {
	h := &mcpHandlers{svc: svc}
	admin.POST("/mcps", h.register)
	admin.GET("/mcps", h.list)
	admin.PATCH("/mcps/:name", h.setEnabled)
	admin.DELETE("/mcps/:name", h.unregister)
}

type connectConfigDTO struct {
	Command   string            `json:"command,omitempty"`
	Arguments []string          `json:"arguments,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	URL       string            `json:"url,omitempty"`
}

type toolSchemaDTO struct {
	Name         string          `json:"name"`
	Description  string          `json:"description,omitempty"`
	InputSchema  json.RawMessage `json:"input_schema,omitempty" swaggertype:"object"`
	OutputSchema json.RawMessage `json:"output_schema,omitempty" swaggertype:"object"`
}

type mcpRegistrationDTO struct {
	Name      string                   `json:"name"`
	Transport string                   `json:"transport"`
	Connect   connectConfigDTO         `json:"connect"`
	Status    string                   `json:"status"`
	Enabled   bool                     `json:"enabled"`
	Tools     map[string]toolSchemaDTO `json:"tools,omitempty"`
}

func mcpRegistrationDTOFrom(reg cache.MCPRegistration) mcpRegistrationDTO {
	var tools map[string]toolSchemaDTO
	if len(reg.Tools) > 0 {
		tools = make(map[string]toolSchemaDTO, len(reg.Tools))
		for name, schema := range reg.Tools {
			tools[name] = toolSchemaDTO{
				Name:         schema.Name,
				Description:  schema.Description,
				InputSchema:  schema.InputSchema,
				OutputSchema: schema.OutputSchema,
			}
		}
	}
	return mcpRegistrationDTO{
		Name:      reg.Name,
		Transport: reg.Transport,
		Connect: connectConfigDTO{
			Command:   reg.Connect.Command,
			Arguments: reg.Connect.Arguments,
			Env:       reg.Connect.Env,
			URL:       reg.Connect.URL,
		},
		Status:  reg.Status,
		Enabled: reg.Enabled,
		Tools:   tools,
	}
}

type registerMCPRequest struct {
	Name      string           `json:"name" binding:"required"`
	Transport string           `json:"transport"`
	Connect   connectConfigDTO `json:"connect"`
	// Enabled is optional; omitting it means enabled (see enabledOrTrue).
	Enabled *bool `json:"enabled"`
}

// setEnabledRequest is the PATCH /admin/mcps/:name body: the operator
// on/off switch, nothing else.
type setEnabledRequest struct {
	Enabled *bool `json:"enabled" binding:"required"`
}

// register handles POST /admin/mcps: connects to and discovers tools on a
// new MCP, then persists the result. If the connect/discover step fails,
// nothing is persisted and the MCP is not made visible to the data plane as
// callable — see ADR-0003.
//
// @Summary      Register an MCP
// @Description  Connects to the MCP, discovers its tools, and makes it immediately callable via the data plane. Persisted only on success.
// @Tags         mcps
// @Accept       json
// @Produce      json
// @Param        request  body      registerMCPRequest  true  "MCP registration"
// @Success      201      {object}  mcpRegistrationDTO
// @Failure      400      {object}  errorResponse
// @Failure      502      {object}  errorResponse
// @Router       /mcps [post]
func (h *mcpHandlers) register(c *gin.Context) {
	var req registerMCPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid_request", Message: err.Error()})
		return
	}

	reg := cache.MCPRegistration{
		Name:      req.Name,
		Transport: req.Transport,
		Connect: cache.ConnectConfig{
			Command:   req.Connect.Command,
			Arguments: req.Connect.Arguments,
			Env:       req.Connect.Env,
			URL:       req.Connect.URL,
		},
		Enabled: enabledOrTrue(req.Enabled),
	}

	registered, err := h.svc.Register(c.Request.Context(), reg)
	if err != nil {
		respondServiceError(c, err)
		return
	}

	c.JSON(http.StatusCreated, mcpRegistrationDTOFrom(registered))
}

// list handles GET /admin/mcps: the live registry state, not the DB — the
// registry is already the read-optimized cache the data plane itself uses.
//
// @Summary      List registered MCPs
// @Tags         mcps
// @Produce      json
// @Success      200  {array}  mcpRegistrationDTO
// @Router       /mcps [get]
func (h *mcpHandlers) list(c *gin.Context) {
	regs := h.svc.List()
	dtos := make([]mcpRegistrationDTO, 0, len(regs))
	for _, reg := range regs {
		dtos = append(dtos, mcpRegistrationDTOFrom(reg))
	}
	c.JSON(http.StatusOK, dtos)
}

// setEnabled handles PATCH /admin/mcps/:name: flips the operator on/off
// switch for an already-registered MCP. It changes the live Registry first
// (so the data plane sees it immediately) then persists, and never
// reconnects or re-discovers the MCP — a disabled MCP stays connected with
// its tools cached, and re-enabling is instant. Disabling is the reversible
// alternative to DELETE, which drops the registration and its schema cache.
//
// @Summary      Enable or disable an MCP
// @Description  Toggles the registration's `enabled` flag in place without reconnecting. A disabled MCP rejects every data-plane call with 403 mcp_disabled.
// @Tags         mcps
// @Accept       json
// @Produce      json
// @Param        name     path      string             true  "MCP name"
// @Param        request  body      setEnabledRequest  true  "Desired state"
// @Success      200      {object}  mcpRegistrationDTO
// @Failure      400      {object}  errorResponse
// @Failure      404      {object}  errorResponse
// @Router       /mcps/{name} [patch]
func (h *mcpHandlers) setEnabled(c *gin.Context) {
	name := c.Param("name")

	var req setEnabledRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid_request", Message: err.Error()})
		return
	}

	reg, err := h.svc.SetEnabled(c.Request.Context(), name, *req.Enabled)
	if err != nil {
		respondServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, mcpRegistrationDTOFrom(reg))
}

// unregister handles DELETE /admin/mcps/:name.
//
// @Summary      Unregister an MCP
// @Tags         mcps
// @Produce      json
// @Param        name  path  string  true  "MCP name"
// @Success      204
// @Failure      404  {object}  errorResponse
// @Router       /mcps/{name} [delete]
func (h *mcpHandlers) unregister(c *gin.Context) {
	if err := h.svc.Unregister(c.Request.Context(), c.Param("name")); err != nil {
		respondServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
