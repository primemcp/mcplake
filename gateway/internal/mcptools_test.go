package internal_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/atsokha/mcplake/auth"
	"github.com/atsokha/mcplake/cache"
	"github.com/atsokha/mcplake/internal"
	"github.com/atsokha/mcplake/mcp"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mcpTestGateway builds a Gateway wired with the given doubles, bound to an
// ephemeral port. These tests drive the MCP surface directly rather than over
// a socket; the transport wiring is covered by mcpendpoint_test.go.
func mcpTestGateway(resolver internal.MCPResolver, policy internal.PolicyEngine) *internal.Gateway {
	return internal.NewGateway(internal.Config{
		DataPlaneAddr: "127.0.0.1:0",
		Authenticator: &fakeAuthenticator{claims: &auth.Claims{Raw: json.RawMessage(`{"sub":"alice"}`)}},
		Policy:        policy,
		Resolver:      resolver,
	})
}

func TestMCPListTools_NamespacesToolsByMCP(t *testing.T) {
	resolver := newFakeResolver()
	resolver.addToolSchema("postgres-ro", cache.ToolSchema{
		Name:         "get_user",
		Description:  "look up a user by id",
		InputSchema:  json.RawMessage(`{"type":"object"}`),
		OutputSchema: json.RawMessage(`{"type":"object"}`),
	})

	g := mcpTestGateway(resolver, &fakePolicyEngine{authorized: true})

	result, err := g.MCPListTools(t.Context(), &auth.Claims{Raw: json.RawMessage(`{}`)})
	require.NoError(t, err)
	require.Len(t, result.Tools, 1)

	tool := result.Tools[0]
	assert.Equal(t, "postgres-ro__get_user", tool.Name)
	assert.Equal(t, "look up a user by id", tool.Description)
	assert.NotNil(t, tool.InputSchema)
	assert.NotNil(t, tool.OutputSchema)
}

// A tool the caller's access policy does not grant must not appear in
// tools/list at all. Discovery and enforcement are the same decision; a tool
// the caller could only ever get a 403 from is noise at best.
func TestMCPListTools_OmitsToolsTheCallerIsNotAuthorizedFor(t *testing.T) {
	resolver := newFakeResolver()
	resolver.addToolSchema("postgres-ro", cache.ToolSchema{Name: "get_user"})
	resolver.addToolSchema("payments", cache.ToolSchema{Name: "refund"})

	// Authorize only postgres-ro's tools.
	policy := &fakePolicyEngine{authorizeFunc: func(_ json.RawMessage, mcpName, _ string) (bool, error) {
		return mcpName == "postgres-ro", nil
	}}

	g := mcpTestGateway(resolver, policy)

	result, err := g.MCPListTools(t.Context(), &auth.Claims{Raw: json.RawMessage(`{}`)})
	require.NoError(t, err)
	require.Len(t, result.Tools, 1)
	assert.Equal(t, "postgres-ro__get_user", result.Tools[0].Name)
}

func TestMCPListTools_SkipsDisabledAndUnreachableMCPs(t *testing.T) {
	resolver := newFakeResolver()
	resolver.addToolSchema("healthy", cache.ToolSchema{Name: "ping"})
	resolver.addToolSchema("switched-off", cache.ToolSchema{Name: "ping"})
	resolver.setDisabled("switched-off")
	resolver.addToolSchema("down", cache.ToolSchema{Name: "ping"})
	resolver.setUnreachable("down")

	g := mcpTestGateway(resolver, &fakePolicyEngine{authorized: true})

	result, err := g.MCPListTools(t.Context(), &auth.Claims{Raw: json.RawMessage(`{}`)})
	require.NoError(t, err)
	require.Len(t, result.Tools, 1)
	assert.Equal(t, "healthy__ping", result.Tools[0].Name)
}

// An MCP that advertises no input schema still has to produce a valid Tool:
// inputSchema is required by the protocol, and a client that receives null
// there cannot call the tool.
func TestMCPListTools_SubstitutesAnEmptyObjectForAMissingInputSchema(t *testing.T) {
	resolver := newFakeResolver()
	resolver.addToolSchema("bare", cache.ToolSchema{Name: "noargs"})

	g := mcpTestGateway(resolver, &fakePolicyEngine{authorized: true})

	result, err := g.MCPListTools(t.Context(), &auth.Claims{Raw: json.RawMessage(`{}`)})
	require.NoError(t, err)
	require.Len(t, result.Tools, 1)

	encoded, err := json.Marshal(result.Tools[0])
	require.NoError(t, err)

	var decoded struct {
		InputSchema  json.RawMessage `json:"inputSchema"`
		OutputSchema json.RawMessage `json:"outputSchema"`
	}
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	assert.JSONEq(t, `{"type":"object"}`, string(decoded.InputSchema))
	assert.Empty(t, decoded.OutputSchema, "an absent output schema must be omitted, not sent as null")
}

func TestMCPListTools_IsDeterministicallyOrdered(t *testing.T) {
	resolver := newFakeResolver()
	resolver.addToolSchema("b-mcp", cache.ToolSchema{Name: "z_tool"})
	resolver.addToolSchema("b-mcp", cache.ToolSchema{Name: "a_tool"})
	resolver.addToolSchema("a-mcp", cache.ToolSchema{Name: "m_tool"})

	g := mcpTestGateway(resolver, &fakePolicyEngine{authorized: true})

	for range 5 {
		result, err := g.MCPListTools(t.Context(), &auth.Claims{Raw: json.RawMessage(`{}`)})
		require.NoError(t, err)
		names := make([]string, 0, len(result.Tools))
		for _, tool := range result.Tools {
			names = append(names, tool.Name)
		}
		assert.Equal(t, []string{"a-mcp__m_tool", "b-mcp__a_tool", "b-mcp__z_tool"}, names)
	}
}

func TestMCPCallTool_RoutesThroughThePipelineAndFilters(t *testing.T) {
	client := &fakeMCPClient{callTool: func(_ context.Context, tool string, args map[string]any) (*mcp.ToolResponse, error) {
		assert.Equal(t, "get_user", tool, "the downstream must be called with the unqualified tool name")
		assert.Equal(t, map[string]any{"id": float64(42)}, args)
		return &mcp.ToolResponse{Raw: toolResultEnvelope(t, `{"name":"alice","ssn":"123"}`)}, nil
	}}

	resolver := newFakeResolver()
	resolver.addToolSchema("postgres-ro", cache.ToolSchema{Name: "get_user"})
	resolver.clients["postgres-ro"] = client

	g := mcpTestGateway(resolver, &fakePolicyEngine{authorized: true, dropFields: []string{"$.ssn"}})

	result, err := g.MCPCallTool(t.Context(), &auth.Claims{Raw: json.RawMessage(`{}`)}, &sdk.CallToolParamsRaw{
		Name:      "postgres-ro__get_user",
		Arguments: json.RawMessage(`{"id":42}`),
	})
	require.NoError(t, err)
	require.False(t, result.IsError)

	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), "alice")
	assert.NotContains(t, string(encoded), "123", "the filter policy's drop_fields must apply here exactly as on /v1/call")
}

// The separator is ambiguous in general, so splitting resolves against the
// registry rather than by parsing: an MCP named "a__b" is still reachable.
func TestMCPCallTool_ResolvesNamesContainingTheSeparator(t *testing.T) {
	called := ""
	client := &fakeMCPClient{callTool: func(_ context.Context, tool string, _ map[string]any) (*mcp.ToolResponse, error) {
		called = tool
		return &mcp.ToolResponse{Raw: toolResultEnvelope(t, `{}`)}, nil
	}}

	resolver := newFakeResolver()
	resolver.addToolSchema("odd__name", cache.ToolSchema{Name: "do__it"})
	resolver.clients["odd__name"] = client

	g := mcpTestGateway(resolver, &fakePolicyEngine{authorized: true})

	_, err := g.MCPCallTool(t.Context(), &auth.Claims{Raw: json.RawMessage(`{}`)}, &sdk.CallToolParamsRaw{
		Name: "odd__name__do__it",
	})
	require.NoError(t, err)
	assert.Equal(t, "do__it", called)
}

func TestMCPCallTool_UnknownToolIsAProtocolError(t *testing.T) {
	g := mcpTestGateway(newFakeResolver(), &fakePolicyEngine{authorized: true})

	_, err := g.MCPCallTool(t.Context(), &auth.Claims{Raw: json.RawMessage(`{}`)}, &sdk.CallToolParamsRaw{
		Name: "nope__missing",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nope__missing")
}

// A caller that is not granted a tool gets a protocol error, not a tool
// result: the call should not have been made, and a model should not treat
// it as something to retry or work around.
func TestMCPCallTool_ForbiddenIsAProtocolError(t *testing.T) {
	resolver := newFakeResolver()
	resolver.addToolSchema("postgres-ro", cache.ToolSchema{Name: "get_user"})
	resolver.clients["postgres-ro"] = &fakeMCPClient{}

	g := mcpTestGateway(resolver, &fakePolicyEngine{authorized: false})

	_, err := g.MCPCallTool(t.Context(), &auth.Claims{Raw: json.RawMessage(`{}`)}, &sdk.CallToolParamsRaw{
		Name: "postgres-ro__get_user",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forbidden")
}

// A downstream that fails is a different matter: the call was legitimate and
// the model can react, so per the MCP spec it comes back as a tool result
// with isError set rather than as a JSON-RPC error.
func TestMCPCallTool_DownstreamFailureIsAToolResultError(t *testing.T) {
	client := &fakeMCPClient{callTool: func(context.Context, string, map[string]any) (*mcp.ToolResponse, error) {
		return nil, assert.AnError
	}}

	resolver := newFakeResolver()
	resolver.addToolSchema("postgres-ro", cache.ToolSchema{Name: "get_user"})
	resolver.clients["postgres-ro"] = client

	g := mcpTestGateway(resolver, &fakePolicyEngine{authorized: true})

	result, err := g.MCPCallTool(t.Context(), &auth.Claims{Raw: json.RawMessage(`{}`)}, &sdk.CallToolParamsRaw{
		Name: "postgres-ro__get_user",
	})
	require.NoError(t, err, "a downstream failure is reported in the result, not as a protocol error")
	require.True(t, result.IsError)

	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), "upstream_error")
}
