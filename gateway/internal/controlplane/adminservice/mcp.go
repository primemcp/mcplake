package adminservice

import (
	"context"
	"fmt"

	"github.com/atsokha/mcplake/cache"
)

// DefaultMCPTransport is applied when a registration request omits the
// transport, matching config.MCPConfig's documented default (docs/CONFIG.md)
// so the admin API and static config.yaml entries behave identically.
const DefaultMCPTransport = "stdio"

// MCPService performs the MCP-registration operations behind
// POST/GET/PATCH/DELETE /admin/mcps. Every write goes through the live
// registry first — so a data-plane caller sees the effect immediately — then
// the durable repository.
type MCPService struct {
	// baseCtx outlives any single admin request: a stdio MCP's subprocess is
	// spawned with exec.CommandContext, so binding registration to the
	// request context would kill the MCP the instant the HTTP/MCP call that
	// registered it returns. It is the same app-lifetime context used to
	// register the config-seeded MCPs at startup.
	baseCtx  context.Context
	registry Registry
	repo     MCPRepository
}

// NewMCPService returns an MCPService backed by registry (live) and repo
// (durable). baseCtx must be the gateway's lifetime context, not a
// request-scoped one — see MCPService.baseCtx.
func NewMCPService(baseCtx context.Context, registry Registry, repo MCPRepository) *MCPService {
	return &MCPService{baseCtx: baseCtx, registry: registry, repo: repo}
}

// List returns every currently registered MCP from the live registry (the
// read-optimized cache the data plane itself uses), not the database.
func (s *MCPService) List() []cache.MCPRegistration {
	return s.registry.List()
}

// Get returns one registration from the live registry, or ErrNotFound.
func (s *MCPService) Get(name string) (cache.MCPRegistration, error) {
	reg, ok := s.registry.Get(name)
	if !ok {
		return cache.MCPRegistration{}, fmt.Errorf("%w: mcp %q is not registered", ErrNotFound, name)
	}
	return reg, nil
}

// Register connects to the MCP, discovers its tools, and makes it callable
// via the data plane, then persists the result. On any failure before
// persistence nothing is stored and the MCP is not made visible to the data
// plane (ErrRegistrationFailed). An empty Name is ErrInvalidRequest. A
// missing Transport defaults to DefaultMCPTransport.
func (s *MCPService) Register(ctx context.Context, reg cache.MCPRegistration) (cache.MCPRegistration, error) {
	if reg.Name == "" {
		return cache.MCPRegistration{}, fmt.Errorf("%w: name is required", ErrInvalidRequest)
	}
	if reg.Transport == "" {
		reg.Transport = DefaultMCPTransport
	}

	// Register with the app-lifetime context, not ctx: the discovered MCP's
	// client (and, for stdio, its subprocess) must outlive this call.
	if err := s.registry.Register(s.baseCtx, reg); err != nil {
		return cache.MCPRegistration{}, fmt.Errorf("%w: %w", ErrRegistrationFailed, err)
	}

	registered, ok := s.registry.Get(reg.Name)
	if !ok {
		return cache.MCPRegistration{}, fmt.Errorf("registration succeeded but registry lookup failed for %q", reg.Name)
	}

	if err := s.repo.Upsert(ctx, registered); err != nil {
		return cache.MCPRegistration{}, fmt.Errorf("persist mcp registration %q: %w", reg.Name, err)
	}
	return registered, nil
}

// SetEnabled flips an already-registered MCP's operator on/off switch in the
// live registry (effective immediately on the data plane) and then persists
// it. It never reconnects or re-discovers the MCP. Unknown name is
// ErrNotFound.
func (s *MCPService) SetEnabled(ctx context.Context, name string, enabled bool) (cache.MCPRegistration, error) {
	reg, ok := s.registry.SetEnabled(name, enabled)
	if !ok {
		return cache.MCPRegistration{}, fmt.Errorf("%w: mcp %q is not registered", ErrNotFound, name)
	}

	if err := s.repo.Upsert(ctx, reg); err != nil {
		return cache.MCPRegistration{}, fmt.Errorf("persist mcp registration %q: %w", name, err)
	}
	return reg, nil
}

// Unregister closes the MCP's client, removes it from the live registry, and
// deletes its persisted row. Unknown name is ErrNotFound.
func (s *MCPService) Unregister(ctx context.Context, name string) error {
	if err := s.registry.Unregister(name); err != nil {
		return fmt.Errorf("%w: %w", ErrNotFound, err)
	}
	if err := s.repo.Delete(ctx, name); err != nil {
		return fmt.Errorf("delete mcp registration %q: %w", name, err)
	}
	return nil
}
