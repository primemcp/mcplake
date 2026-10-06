package cache

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/primemcp/mcplake/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// activeReg builds a StatusActive registration wired to client with the
// given tool names cached, and stores it directly (bypassing Register's
// connect step, which the fake client would otherwise satisfy anyway).
func activeReg(r *Registry, name string, client MCPClient, tools ...string) {
	toolMap := make(map[string]ToolSchema, len(tools))
	for _, t := range tools {
		toolMap[t] = ToolSchema{Name: t}
	}
	r.set(MCPRegistration{
		Name:      name,
		Transport: "stdio",
		Status:    StatusActive,
		Tools:     toolMap,
		Client:    client,
		Enabled:   true,
	})
}

func TestRefreshTools_AddsAndRemovesTools(t *testing.T) {
	r := NewRegistry()
	client := &fakeMCPClient{tools: []mcp.ToolSchema{{Name: "get_user"}}}
	activeReg(r, "pg", client, "get_user")

	// Downstream now advertises a second tool and drops the first.
	client.tools = []mcp.ToolSchema{{Name: "list_users"}}

	require.NoError(t, r.RefreshTools(context.Background(), "pg"))

	assert.True(t, r.HasTool("pg", "list_users"), "a newly-advertised tool becomes callable")
	assert.False(t, r.HasTool("pg", "get_user"), "a dropped tool stops being callable")
}

func TestRefreshTools_PreservesStatusEnabledAndClient(t *testing.T) {
	r := NewRegistry()
	client := &fakeMCPClient{tools: []mcp.ToolSchema{{Name: "a"}}}
	activeReg(r, "pg", client, "a")
	r.SetEnabled("pg", false)

	client.tools = []mcp.ToolSchema{{Name: "a"}, {Name: "b"}}
	require.NoError(t, r.RefreshTools(context.Background(), "pg"))

	reg, ok := r.Get("pg")
	require.True(t, ok)
	assert.Equal(t, StatusActive, reg.Status)
	assert.False(t, reg.Enabled, "operator intent is untouched by a refresh")
	assert.Same(t, client, reg.Client.(*fakeMCPClient))
	assert.Len(t, reg.Tools, 2)
}

func TestRefreshTools_ListErrorLeavesPreviousSchemaIntact(t *testing.T) {
	r := NewRegistry()
	client := &fakeMCPClient{tools: []mcp.ToolSchema{{Name: "get_user"}}}
	activeReg(r, "pg", client, "get_user")

	client.listErr = errors.New("mcp busy")

	err := r.RefreshTools(context.Background(), "pg")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "refresh tools for \"pg\"")
	assert.True(t, r.HasTool("pg", "get_user"), "the previous schema survives a failed refresh")
	reg, _ := r.Get("pg")
	assert.Equal(t, StatusActive, reg.Status)
}

func TestRefreshTools_NoOpForUnknownNonActiveOrClientless(t *testing.T) {
	r := NewRegistry()

	assert.NoError(t, r.RefreshTools(context.Background(), "absent"))

	r.set(MCPRegistration{Name: "connecting", Status: StatusConnecting, Client: &fakeMCPClient{}})
	assert.NoError(t, r.RefreshTools(context.Background(), "connecting"))

	r.set(MCPRegistration{Name: "noclient", Status: StatusActive})
	assert.NoError(t, r.RefreshTools(context.Background(), "noclient"))
}

func TestRefreshTools_DiscardsResultWhenClientReplacedMidRefresh(t *testing.T) {
	r := NewRegistry()
	oldClient := &fakeMCPClient{tools: []mcp.ToolSchema{{Name: "old"}}}
	activeReg(r, "pg", oldClient, "old")

	// The ListTools call is where a race would land; simulate the swap
	// happening during it.
	newClient := &fakeMCPClient{tools: []mcp.ToolSchema{{Name: "new"}}}
	oldClient.onListTools = func() {
		activeReg(r, "pg", newClient, "new")
	}
	oldClient.tools = []mcp.ToolSchema{{Name: "stale"}}

	require.NoError(t, r.RefreshTools(context.Background(), "pg"))

	// The refresh discovered against oldClient; by swap time the registration
	// points at newClient, so the "stale" result must be dropped.
	assert.True(t, r.HasTool("pg", "new"))
	assert.False(t, r.HasTool("pg", "stale"))
}

func TestRefreshActive_RefreshesEveryActiveMCPAndReportsPerMCP(t *testing.T) {
	r := NewRegistry()
	good := &fakeMCPClient{tools: []mcp.ToolSchema{{Name: "x"}}}
	bad := &fakeMCPClient{listErr: errors.New("down")}
	activeReg(r, "good", good, "x")
	activeReg(r, "bad", bad, "y")
	r.set(MCPRegistration{Name: "idle", Status: StatusUnreachable})

	good.tools = []mcp.ToolSchema{{Name: "x"}, {Name: "z"}}

	results := r.RefreshActive(context.Background())

	byName := map[string]error{}
	for _, res := range results {
		byName[res.Name] = res.Err
	}
	require.Len(t, byName, 2, "unreachable MCP is skipped")
	assert.NoError(t, byName["good"])
	assert.Error(t, byName["bad"])
	assert.True(t, r.HasTool("good", "z"), "the good MCP was refreshed despite the bad one failing")
}

func TestRefreshTools_ConcurrentWithGetIsRaceFree(t *testing.T) {
	r := NewRegistry()
	client := &fakeMCPClient{tools: []mcp.ToolSchema{{Name: "a"}}}
	activeReg(r, "pg", client, "a")

	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_, _ = r.Get("pg")
				r.HasTool("pg", "a")
			}
		}
	}()

	for range 50 {
		require.NoError(t, r.RefreshTools(context.Background(), "pg"))
	}
	close(stop)
	wg.Wait()
}
