package cache

import (
	"context"
	"testing"

	"github.com/primemcp/mcplake/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry_Resolve_ReturnsClientForActiveRegistration(t *testing.T) {
	fake := &fakeMCPClient{tools: []mcp.ToolSchema{{Name: "get_user"}}}
	withFakeNewMCPClient(t, func(context.Context, mcp.Config) (MCPClient, error) { return fake, nil })

	r := NewRegistry()
	require.NoError(t, r.Register(context.Background(), MCPRegistration{
		Name: "postgres-ro", Transport: "stdio", Connect: ConnectConfig{Command: "x"},
	}))

	client, ok := r.Resolve("postgres-ro")
	require.True(t, ok)
	assert.Same(t, fake, client)
}

func TestRegistry_Resolve_UnknownMCPReturnsNotOK(t *testing.T) {
	r := NewRegistry()
	_, ok := r.Resolve("does-not-exist")
	assert.False(t, ok)
}

func TestRegistry_Resolve_UnreachableMCPReturnsNotOKEvenWithStaleClient(t *testing.T) {
	r := NewRegistry()
	// Directly seed a registration in a state Register itself would never
	// produce today (unreachable with a leftover client/tools) - this is
	// exactly the defensive case the ticket calls for: a stale client must
	// never be resolved once an MCP is no longer active.
	r.set(MCPRegistration{
		Name:   "flaky-mcp",
		Status: StatusUnreachable,
		Tools:  map[string]ToolSchema{"get_user": {Name: "get_user"}},
		Client: &fakeMCPClient{},
	})

	_, ok := r.Resolve("flaky-mcp")
	assert.False(t, ok)
}

func TestRegistry_HasTool_UnreachableMCPWithStaleToolsIsNotCallable(t *testing.T) {
	r := NewRegistry()
	r.set(MCPRegistration{
		Name:   "flaky-mcp",
		Status: StatusUnreachable,
		Tools:  map[string]ToolSchema{"get_user": {Name: "get_user"}},
	})

	assert.False(t, r.HasTool("flaky-mcp", "get_user"), "a stale tool list on an unreachable MCP must not be treated as callable")
}

func TestRegistry_Get_ToolsMapIsDefensivelyCopied(t *testing.T) {
	r := NewRegistry()
	r.set(MCPRegistration{
		Name:   "postgres-ro",
		Status: StatusActive,
		Tools:  map[string]ToolSchema{"get_user": {Name: "get_user"}},
	})

	got, ok := r.Get("postgres-ro")
	require.True(t, ok)
	got.Tools["injected"] = ToolSchema{Name: "injected"}

	fresh, _ := r.Get("postgres-ro")
	assert.NotContains(t, fresh.Tools, "injected", "mutating the returned Tools map must not affect Registry state")
}

func TestRegistry_List_ToolsMapIsDefensivelyCopied(t *testing.T) {
	r := NewRegistry()
	r.set(MCPRegistration{
		Name:   "postgres-ro",
		Status: StatusActive,
		Tools:  map[string]ToolSchema{"get_user": {Name: "get_user"}},
	})

	list := r.List()
	list[0].Tools["injected"] = ToolSchema{Name: "injected"}

	fresh, _ := r.Get("postgres-ro")
	assert.NotContains(t, fresh.Tools, "injected", "mutating a List() element's Tools map must not affect Registry state")
}
