// Package cache implements the MCP Registry: the gateway's in-memory record
// of every registered downstream MCP and the tools it advertises. See
// docs/architecture/components.rst#mcp-registry-cache-extended and ADR-0003.
package cache

import (
	"context"
	"encoding/json"
	"fmt"
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

// Transport names for a registration, re-exported from mcp so callers that
// already depend on cache (config, the control plane) don't each need a
// direct mcp import for a string constant. mcp owns them because mcp is
// what dials them; see ADR-0017.
const (
	TransportStdio = mcp.TransportStdio
	TransportHTTP  = mcp.TransportHTTP
	TransportSSE   = mcp.TransportSSE
)

// ConnectConfig holds the transport-specific details needed to connect to an
// MCP: Command/Arguments/Env for the stdio transport, or URL for sse/http.
// Env is additional environment for the subprocess, beyond the gateway's
// documented base set (see mcp.Config.Env) — the MCP does not otherwise
// inherit the gateway's own environment.
type ConnectConfig struct {
	Command   string
	Arguments []string
	Env       map[string]string
	URL       string
}

// ToolSchema describes one tool a registered MCP advertises via tools/list.
type ToolSchema struct {
	Name string
	// Description is the tool's human-readable description, as the
	// downstream advertised it. The data-plane MCP endpoint re-advertises
	// it to its own callers, which is how a client's model picks a tool;
	// see ADR-0021. Empty when the MCP doesn't provide one.
	Description  string
	InputSchema  json.RawMessage
	OutputSchema json.RawMessage // nil when the MCP doesn't provide one
}

// toolMapFrom converts a discovery result into the by-name map a
// registration stores. Every path that discovers tools -- Register,
// RefreshTools and the health loop's reconnect -- goes through here, so a
// field added to ToolSchema reaches all three or none.
func toolMapFrom(tools []mcp.ToolSchema) map[string]ToolSchema {
	toolMap := make(map[string]ToolSchema, len(tools))
	for _, t := range tools {
		toolMap[t.Name] = ToolSchema{
			Name:         t.Name,
			Description:  t.Description,
			InputSchema:  t.InputSchema,
			OutputSchema: t.OutputSchema,
		}
	}
	return toolMap
}

// MCPClient is the connection surface Registry needs from a downstream MCP
// client, satisfied by *mcp.Client. Defined here (consumer-side, per the
// project's small-interfaces convention) so tests can substitute a fake
// without spawning a real subprocess; see Register in register.go.
type MCPClient interface {
	ListTools(ctx context.Context) ([]mcp.ToolSchema, error)
	CallTool(ctx context.Context, tool string, args map[string]any) (*mcp.ToolResponse, error)
	// Ping reports whether the session is still usable. See
	// Registry.CheckHealth (health.go) and ADR-0019.
	Ping(ctx context.Context) error
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
	// Enabled is the operator's on/off switch for this MCP, independent of
	// Status: a disabled MCP stays connected (Status may still be
	// StatusActive) with its tools cached, so re-enabling needs no
	// reconnect, but the data-plane pipeline refuses to route calls to it
	// (see #95). Status reflects connection health and is owned by the
	// gateway; Enabled reflects operator intent. It defaults to true at
	// every boundary that constructs a registration (config, persistence,
	// admin API).
	Enabled bool
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

// Get returns the registration stored under name, if any. The returned
// value's Tools map is a defensive copy (see MCPRegistration.clone) — the
// caller cannot mutate Registry-internal state through it.
func (r *Registry) Get(name string) (MCPRegistration, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	reg, ok := r.byName[name]
	return reg.clone(), ok
}

// HasTool reports whether mcp is currently active and advertises tool. An
// MCP that isn't StatusActive never returns true here, even if it has a
// tool list left over from a previous successful registration — a stale
// tool list on an unreachable MCP must not be treated as callable.
func (r *Registry) HasTool(mcp, tool string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	reg, ok := r.byName[mcp]
	if !ok || reg.Status != StatusActive {
		return false
	}
	_, ok = reg.Tools[tool]
	return ok
}

// Resolve returns the live client for mcp, but only if it is currently
// StatusActive — the gateway pipeline uses this to get a client to call
// through, and must never be handed a client for an unreachable or unknown
// MCP.
func (r *Registry) Resolve(mcp string) (MCPClient, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	reg, ok := r.byName[mcp]
	if !ok || reg.Status != StatusActive {
		return nil, false
	}
	return reg.Client, true
}

// Disabled reports whether mcp is registered but administratively disabled
// (Enabled == false). A disabled registration keeps its cached tools and
// live client — so re-enabling needs no reconnect — but the data-plane
// pipeline must refuse to route calls to it (see #95). An unknown MCP
// reports false: it is not "disabled", it is absent, which callers detect
// separately via Resolve.
func (r *Registry) Disabled(mcp string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	reg, ok := r.byName[mcp]
	return ok && !reg.Enabled
}

// Registered reports whether name is in the Registry at all, whatever its
// status. Resolve answers the narrower "is there a client I may call
// through"; this answers "does this MCP exist", which is what tells a
// caller whether a refused call means an unknown name or a known one whose
// downstream is currently unreachable (ADR-0019).
func (r *Registry) Registered(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.byName[name]
	return ok
}

// SetEnabled flips the operator on/off switch for name in place, without
// touching its Status, tools, or live client — so toggling a running MCP
// off and back on never reconnects or re-discovers it (see #95). It returns
// the updated registration and true, or a zero value and false when no MCP
// is registered under name.
func (r *Registry) SetEnabled(name string, enabled bool) (MCPRegistration, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	reg, ok := r.byName[name]
	if !ok {
		return MCPRegistration{}, false
	}
	reg.Enabled = enabled
	r.byName[name] = reg
	return reg.clone(), true
}

// List returns a defensive copy of every registration currently stored:
// callers cannot mutate Registry-internal state through the returned slice
// or its elements' Tools maps.
func (r *Registry) List() []MCPRegistration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]MCPRegistration, 0, len(r.byName))
	for _, reg := range r.byName {
		out = append(out, reg.clone())
	}
	return out
}

// clone returns a copy of reg safe to hand to callers outside the lock:
// the Tools map is copied, since a map is a reference type and a plain
// struct copy would otherwise still share the same underlying map with
// Registry-internal state. Client is intentionally left shared — callers
// that get a client via Resolve need the actual live connection, not a
// copy of it.
func (reg MCPRegistration) clone() MCPRegistration {
	if reg.Tools == nil {
		return reg
	}
	tools := make(map[string]ToolSchema, len(reg.Tools))
	for name, schema := range reg.Tools {
		tools[name] = schema
	}
	reg.Tools = tools
	return reg
}

// set stores reg under its Name, replacing any existing entry in a single
// atomic map write — a concurrent Get/HasTool/List either observes the
// complete previous value or the complete new one, never a mix of the two.
// It returns whatever was previously stored under that name, if anything,
// so Register (register.go) can close a superseded client after the swap.
//
// Unexported: it exists so this package's own tests can populate a Registry
// directly without a real MCP connection; Registry.Register is the
// production write path.
func (r *Registry) set(reg MCPRegistration) (previous MCPRegistration, hadPrevious bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	previous, hadPrevious = r.byName[reg.Name]
	r.byName[reg.Name] = reg
	return previous, hadPrevious
}

// setIfAbsent stores reg under its Name only if nothing is currently
// registered under that name; otherwise it leaves the existing entry
// untouched. Used to record a fresh (never-before-seen) MCP as unreachable
// on its first failed registration attempt, without clobbering an existing,
// possibly still-working registration when a re-registration attempt fails
// — see Register's make-before-break contract in register.go and ADR-0003.
func (r *Registry) setIfAbsent(reg MCPRegistration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byName[reg.Name]; exists {
		return
	}
	r.byName[reg.Name] = reg
}

// Unregister removes name from the Registry and closes its client. Once
// this returns, new calls referencing name are rejected as not-found
// immediately. It does not forcibly interrupt an in-flight call already
// holding a reference to the client (e.g. via a prior Resolve) — that call
// is allowed to finish; only new lookups are affected.
func (r *Registry) Unregister(name string) error {
	r.mu.Lock()
	reg, ok := r.byName[name]
	if !ok {
		r.mu.Unlock()
		return fmt.Errorf("cache: %q is not registered", name)
	}
	delete(r.byName, name)
	r.mu.Unlock()

	if reg.Client != nil {
		return reg.Client.Close()
	}
	return nil
}
