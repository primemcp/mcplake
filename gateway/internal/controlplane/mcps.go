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
	admin.DELETE("/mcps/:name", h.unregister)
}

type connectConfigDTO struct {
	Command   string   `json:"command,omitempty"`
	Arguments []string `json:"arguments,omitempty"`
	URL       string   `json:"url,omitempty"`
}

type toolSchemaDTO struct {
	Name         string          `json:"name"`
	InputSchema  json.RawMessage `json:"input_schema,omitempty"`
	OutputSchema json.RawMessage `json:"output_schema,omitempty"`
}

type mcpRegistrationDTO struct {
	Name      string                   `json:"name"`
	Transport string                   `json:"transport"`
	Connect   connectConfigDTO         `json:"connect"`
	Status    string                   `json:"status"`
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
		Status: reg.Status,
		Tools:  tools,
	}
}

type registerMCPRequest struct {
	Name      string           `json:"name" binding:"required"`
	Transport string           `json:"transport"`
	Connect   connectConfigDTO `json:"connect"`
}

// register handles POST /admin/mcps: connects to and discovers tools on a
// new MCP (via registry.Register), then persists the result. If Register
// fails, nothing is persisted and the MCP is not made visible to the data
// plane as callable — see ADR-0003.
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
func (h *mcpHandlers) list(c *gin.Context) {
	regs := h.registry.List()
	dtos := make([]mcpRegistrationDTO, 0, len(regs))
	for _, reg := range regs {
		dtos = append(dtos, mcpRegistrationDTOFrom(reg))
	}
	c.JSON(http.StatusOK, dtos)
}

// unregister handles DELETE /admin/mcps/:name.
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
