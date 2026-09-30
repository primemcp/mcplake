// Package adminservice is the transport-agnostic application layer behind
// the control-plane admin API: the MCP-registration and access/filter-policy
// operations, with their write-through ordering (live in-memory Registry /
// PolicyStore first, then the durable store) and validation in one place.
//
// The Gin HTTP handlers in package controlplane and the MCP control server
// in package adminmcp are both thin adapters over these services, so the two
// surfaces cannot drift. See
// docs/architecture/decisions/0011-mcp-control-server.rst.
package adminservice

import (
	"context"
	"errors"

	"github.com/atsokha/mcplake/cache"
	"github.com/atsokha/mcplake/router"
)

// Sentinel errors returned by the services, wrapped with context via
// fmt.Errorf("%w: ..."). Adapters classify failures with errors.Is and map
// them to their transport's error shape (HTTP status, MCP tool error).
// An error matching none of these is an unexpected internal failure.
var (
	// ErrInvalidRequest is a malformed request the caller can fix (e.g. a
	// missing required field).
	ErrInvalidRequest = errors.New("adminservice: invalid request")
	// ErrNotFound is returned when the named MCP or policy does not exist.
	ErrNotFound = errors.New("adminservice: not found")
	// ErrRegistrationFailed is returned when connecting to or discovering
	// tools on a downstream MCP fails; nothing is persisted.
	ErrRegistrationFailed = errors.New("adminservice: mcp registration failed")
	// ErrInvalidPolicy is returned when the durable store rejects a policy
	// (e.g. a claim rule that fails to compile); nothing is persisted.
	ErrInvalidPolicy = errors.New("adminservice: invalid policy")
)

// Registry is the behavior the services need from the live in-memory MCP
// registry, satisfied by *cache.Registry.
type Registry interface {
	Register(ctx context.Context, reg cache.MCPRegistration) error
	Unregister(name string) error
	Get(name string) (cache.MCPRegistration, bool)
	List() []cache.MCPRegistration
	SetEnabled(name string, enabled bool) (cache.MCPRegistration, bool)
}

// MCPRepository is the behavior the services need from the durable MCP
// registration store, satisfied by *persistence.MCPRegistrationRepo.
type MCPRepository interface {
	Upsert(ctx context.Context, reg cache.MCPRegistration) error
	Delete(ctx context.Context, name string) error
}

// AccessPolicyRepository is the durable access-policy store, satisfied by
// *persistence.AccessPolicyRepo.
type AccessPolicyRepository interface {
	Upsert(ctx context.Context, policy router.AccessPolicy) error
	Get(ctx context.Context, name string) (router.AccessPolicy, bool, error)
	List(ctx context.Context) ([]router.AccessPolicy, error)
	Delete(ctx context.Context, name string) error
}

// FilterPolicyRepository is the durable filter-policy store, satisfied by
// *persistence.FilterPolicyRepo.
type FilterPolicyRepository interface {
	Upsert(ctx context.Context, policy router.FilterPolicy) error
	Get(ctx context.Context, name string) (router.FilterPolicy, bool, error)
	List(ctx context.Context) ([]router.FilterPolicy, error)
	Delete(ctx context.Context, name string) error
}

// Reloader atomically refreshes the live policy engine from the current
// state of both policy repositories, satisfied by
// *controlplane.PolicyReloader.
type Reloader interface {
	Refresh(ctx context.Context) error
}

// Services bundles the three admin services so a single value can be handed
// to every adapter.
type Services struct {
	MCP    *MCPService
	Access *AccessPolicyService
	Filter *FilterPolicyService
}

// New wires the services from the same Registry / repositories / reloader
// the data plane reads, so an admin write is live immediately. baseCtx is
// the gateway's lifetime context: MCP registrations made through the admin
// surfaces are bound to it, not to the request that triggered them (see
// MCPService.baseCtx).
func New(
	baseCtx context.Context,
	registry Registry,
	mcpRepo MCPRepository,
	accessRepo AccessPolicyRepository,
	filterRepo FilterPolicyRepository,
	reloader Reloader,
) *Services {
	return &Services{
		MCP:    NewMCPService(baseCtx, registry, mcpRepo),
		Access: NewAccessPolicyService(accessRepo, reloader),
		Filter: NewFilterPolicyService(filterRepo, reloader),
	}
}
