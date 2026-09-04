// Package cache implements the MCP Registry: the gateway's in-memory record
// of every registered downstream MCP and the tools it advertises. See
// docs/architecture/components.md#mcp-registry-cache-extended and ADR-0003.
package cache

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/atsokha/mcplake/mcp"
)

// Registration status values. See ADR-0003 for the state machine: a
// registration starts Connecting, becomes Active on a successful tools/list,
// or Unreachable if connecting/discovery fails.
const (
	StatusConnecting  = "connecting"
	StatusActive      = "active"
	StatusUnreachable = "unreachable"
)

// ConnectConfig holds the transport-specific details needed to connect to an
// MCP: Command/Arguments for the stdio transport, or URL for sse/http.
type ConnectConfig struct {
	Command   string
	Arguments []string
	URL       string
}

// ToolSchema describes one tool a registered MCP advertises via tools/list.
type ToolSchema struct {
	Name         string
	InputSchema  json.RawMessage
	OutputSchema json.RawMessage // nil when the MCP doesn't provide one
}

// MCPClient is the connection surface Registry needs from a downstream MCP
// client, satisfied by *mcp.Client. Defined here (consumer-side, per the
// project's small-interfaces convention) so tests can substitute a fake
// without spawning a real subprocess; see Register in register.go.
type MCPClient interface {
	ListTools(ctx context.Context) ([]mcp.ToolSchema, error)
	CallTool(ctx context.Context, tool string, args map[string]any) (*mcp.ToolResponse, error)
	Close() error
}

// MCPRegistration is a downstream MCP the gateway knows about: how to reach
// it, its current status, and (once Status is StatusActive) the tools it
// advertised at last successful discovery plus the live client to call
// through.
type MCPRegistration struct {
	Name      string
	Transport string // "stdio" | "sse" | "http"
	Connect   ConnectConfig
	Status    string
	Tools     map[string]ToolSchema
	// Client is the live connection for this registration, non-nil only
	// when Status is StatusActive.
	Client MCPClient
}

// Registry is the gateway's concurrency-safe, in-memory table of
// MCPRegistrations. The production write path (connect, discover tools,
// activate) is Registry.Register, added in ticket #17; this type currently
// exposes only the read surface plus an unexported test seam.
type Registry struct {
	mu     sync.RWMutex
	byName map[string]MCPRegistration
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{byName: make(map[string]MCPRegistration)}
}

// Get returns the registration stored under name, if any.
func (r *Registry) Get(name string) (MCPRegistration, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	reg, ok := r.byName[name]
	return reg, ok
}

// HasTool reports whether mcp is registered and advertises tool. It does not
// (yet) distinguish an Active registration from an Unreachable one with a
// stale tool list — that visibility distinction is ticket #20's job.
func (r *Registry) HasTool(mcp, tool string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	reg, ok := r.byName[mcp]
	if !ok {
		return false
	}
	_, ok = reg.Tools[tool]
	return ok
}

// List returns a defensive copy of every registration currently stored.
func (r *Registry) List() []MCPRegistration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]MCPRegistration, 0, len(r.byName))
	for _, reg := range r.byName {
		out = append(out, reg)
	}
	return out
}

// set stores reg under its Name, replacing any existing entry. Unexported:
// it exists so this package's own tests can populate a Registry directly
// without a real MCP connection. Registry.Register (ticket #17) is the
// production write path and will call this internally once discovery
// succeeds.
func (r *Registry) set(reg MCPRegistration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byName[reg.Name] = reg
}
