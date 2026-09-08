package controlplane

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/atsokha/mcplake/cache"
	"github.com/gin-gonic/gin"
)

// defaultMCPTransport matches config.MCPConfig's documented default (see
// docs/CONFIG.md) so the admin API and static config.yaml entries behave
// identically when transport is omitted.
const defaultMCPTransport = "stdio"

// mcpRegistry is the behavior RegisterMCPRoutes needs from an MCP registry,
// satisfied by *cache.Registry. Defined here (consumer-side) so handler
// tests can substitute a fake without a real MCP subprocess.
type mcpRegistry interface {
	Register(ctx context.Context, reg cache.MCPRegistration) error
	Unregister(name string) error
	Get(name string) (cache.MCPRegistration, bool)
	List() []cache.MCPRegistration
	SetEnabled(name string, enabled bool) (cache.MCPRegistration, bool)
}

// mcpRepository is the behavior RegisterMCPRoutes needs from the durable
// MCP registration store, satisfied by *persistence.MCPRegistrationRepo.
type mcpRepository interface {
	Upsert(ctx context.Context, reg cache.MCPRegistration) error
	Delete(ctx context.Context, name string) error
}

type mcpHandlers struct {
	registry mcpRegistry
	repo     mcpRepository
}

// RegisterMCPRoutes registers POST/GET /admin/mcps and DELETE
// /admin/mcps/:name onto admin, backed by registry (the live in-memory MCP
// Registry) and repo (durable storage). Every successful write goes through
// registry first — so a caller of the data plane sees the effect
// immediately — then repo, so it survives a restart.
func RegisterMCPRoutes(admin *gin.RouterGroup, registry mcpRegistry, repo mcpRepository) {
	h := &mcpHandlers{registry: registry, repo: repo}
	admin.POST("/mcps", h.register)
	admin.GET("/mcps", h.list)
	admin.PATCH("/mcps/:name", h.setEnabled)
	admin.DELETE("/mcps/:name", h.unregister)
}

type connectConfigDTO struct {
	Command   string   `json:"command,omitempty"`
	Arguments []string `json:"arguments,omitempty"`
	URL       string   `json:"url,omitempty"`
}

type toolSchemaDTO struct {
	Name         string          `json:"name"`
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
// new MCP (via registry.Register), then persists the result. If Register
// fails, nothing is persisted and the MCP is not made visible to the data
// plane as callable — see ADR-0003.
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

	transport := req.Transport
	if transport == "" {
		transport = defaultMCPTransport
	}

	reg := cache.MCPRegistration{
		Name:      req.Name,
		Transport: transport,
		Connect: cache.ConnectConfig{
			Command:   req.Connect.Command,
			Arguments: req.Connect.Arguments,
			URL:       req.Connect.URL,
		},
		Enabled: enabledOrTrue(req.Enabled),
	}

	ctx := c.Request.Context()
	if err := h.registry.Register(ctx, reg); err != nil {
		c.JSON(http.StatusBadGateway, errorResponse{Error: "registration_failed", Message: err.Error()})
		return
	}

	registered, ok := h.registry.Get(req.Name)
	if !ok {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "internal_error", Message: "registration succeeded but registry lookup failed"})
		return
	}

	if err := h.repo.Upsert(ctx, registered); err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "internal_error", Message: err.Error()})
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
	regs := h.registry.List()
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

	reg, ok := h.registry.SetEnabled(name, *req.Enabled)
	if !ok {
		c.JSON(http.StatusNotFound, errorResponse{Error: "not_found", Message: "mcp is not registered"})
		return
	}

	if err := h.repo.Upsert(c.Request.Context(), reg); err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "internal_error", Message: err.Error()})
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
	name := c.Param("name")

	if err := h.registry.Unregister(name); err != nil {
		c.JSON(http.StatusNotFound, errorResponse{Error: "not_found", Message: err.Error()})
		return
	}

	if err := h.repo.Delete(c.Request.Context(), name); err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "internal_error", Message: err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}
