package internal_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/atsokha/mcplake/auth"
	"github.com/atsokha/mcplake/internal"
	"github.com/atsokha/mcplake/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validClaims() *auth.Claims {
	return &auth.Claims{Subject: "user-1", Raw: json.RawMessage(`{"role":"user"}`)}
}

func TestToolCall_InvalidTokenReturns401(t *testing.T) {
	resolver := newFakeResolver()
	resolver.addTool("postgres-ro", "get_user", &fakeMCPClient{})

	addr, cleanup := startTestGatewayWithConfig(t, internal.Config{
		Authenticator: &fakeAuthenticator{err: auth.ErrUnauthorized},
		Policy:        &fakePolicyEngine{authorized: true},
		Resolver:      resolver,
	})
	defer cleanup()

	resp := postToolCall(t, addr, `{"mcp":"postgres-ro","tool":"get_user"}`, "Bearer bad.token")
	body := decodeJSONBody(t, resp)

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.Equal(t, "unauthorized", body["error"])
}

func TestToolCall_AuthorizePolicyErrorReturns500(t *testing.T) {
	resolver := newFakeResolver()
	resolver.addTool("postgres-ro", "get_user", &fakeMCPClient{})

	addr, cleanup := startTestGatewayWithConfig(t, internal.Config{
		Authenticator: &fakeAuthenticator{claims: validClaims()},
		Policy:        &fakePolicyEngine{authorizeErr: errors.New("policy engine misconfigured")},
		Resolver:      resolver,
	})
	defer cleanup()

	resp := postToolCall(t, addr, `{"mcp":"postgres-ro","tool":"get_user"}`, "Bearer token")
	body := decodeJSONBody(t, resp)

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	assert.Equal(t, "internal_error", body["error"])
}

func TestToolCall_FieldsToRemovePolicyErrorReturns500(t *testing.T) {
	resolver := newFakeResolver()
	resolver.addTool("postgres-ro", "get_user", &fakeMCPClient{
		callTool: func(context.Context, string, map[string]any) (*mcp.ToolResponse, error) {
			return &mcp.ToolResponse{Raw: json.RawMessage(`{"id":1}`)}, nil
		},
	})

	addr, cleanup := startTestGatewayWithConfig(t, internal.Config{
		Authenticator: &fakeAuthenticator{claims: validClaims()},
		Policy: &fakePolicyEngine{
			authorized: true,
			fieldsErr:  errors.New("policy engine misconfigured"),
		},
		Resolver: resolver,
	})
	defer cleanup()

	resp := postToolCall(t, addr, `{"mcp":"postgres-ro","tool":"get_user"}`, "Bearer token")
	body := decodeJSONBody(t, resp)

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	assert.Equal(t, "internal_error", body["error"])
}

func TestToolCall_UnauthorizedReturns403(t *testing.T) {
	resolver := newFakeResolver()
	resolver.addTool("postgres-ro", "get_user", &fakeMCPClient{})

	addr, cleanup := startTestGatewayWithConfig(t, internal.Config{
		Authenticator: &fakeAuthenticator{claims: validClaims()},
		Policy:        &fakePolicyEngine{authorized: false},
		Resolver:      resolver,
	})
	defer cleanup()

	resp := postToolCall(t, addr, `{"mcp":"postgres-ro","tool":"get_user"}`, "Bearer token")
	body := decodeJSONBody(t, resp)

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	assert.Equal(t, "forbidden", body["error"])
}

func TestToolCall_UnknownMCPReturns404(t *testing.T) {
	addr, cleanup := startTestGatewayWithConfig(t, internal.Config{
		Authenticator: &fakeAuthenticator{claims: validClaims()},
		Policy:        &fakePolicyEngine{authorized: true},
		Resolver:      newFakeResolver(), // empty: nothing registered
	})
	defer cleanup()

	resp := postToolCall(t, addr, `{"mcp":"does-not-exist","tool":"get_user"}`, "Bearer token")
	body := decodeJSONBody(t, resp)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Equal(t, "mcp_not_found", body["error"])
}

// A registered MCP whose downstream is currently unreachable is a
// different answer from an unknown one, and a recoverable one: the health
// loop is already trying to reconnect it (ADR-0019). Returning 404 sent an
// operator looking for a missing registration that was in fact fine.
func TestToolCall_UnreachableMCPReturns503(t *testing.T) {
	resolver := newFakeResolver()
	resolver.setUnreachable("postgres-ro") // registered, but Resolve has no client

	addr, cleanup := startTestGatewayWithConfig(t, internal.Config{
		Authenticator: &fakeAuthenticator{claims: validClaims()},
		Policy:        &fakePolicyEngine{authorized: true},
		Resolver:      resolver,
	})
	defer cleanup()

	resp := postToolCall(t, addr, `{"mcp":"postgres-ro","tool":"get_user"}`, "Bearer token")
	body := decodeJSONBody(t, resp)

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	assert.Equal(t, "mcp_unavailable", body["error"],
		"a registered-but-down MCP must be distinguishable from one that does not exist")
}

func TestToolCall_UnknownToolReturns404(t *testing.T) {
	resolver := newFakeResolver()
	resolver.addTool("postgres-ro", "get_user", &fakeMCPClient{}) // only get_user exists

	addr, cleanup := startTestGatewayWithConfig(t, internal.Config{
		Authenticator: &fakeAuthenticator{claims: validClaims()},
		Policy:        &fakePolicyEngine{authorized: true},
		Resolver:      resolver,
	})
	defer cleanup()

	resp := postToolCall(t, addr, `{"mcp":"postgres-ro","tool":"delete_everything"}`, "Bearer token")
	body := decodeJSONBody(t, resp)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Equal(t, "tool_not_found", body["error"])
}

func TestToolCall_DisabledMCPReturns403MCPDisabled(t *testing.T) {
	resolver := newFakeResolver()
	// The MCP is fully registered — client and tool present — but disabled.
	resolver.addTool("postgres-ro", "get_user", &fakeMCPClient{
		callTool: func(context.Context, string, map[string]any) (*mcp.ToolResponse, error) {
			t.Fatal("downstream MCP must not be called when the MCP is disabled")
			return nil, nil
		},
	})
	resolver.setDisabled("postgres-ro")

	addr, cleanup := startTestGatewayWithConfig(t, internal.Config{
		Authenticator: &fakeAuthenticator{claims: validClaims()},
		Policy:        &fakePolicyEngine{authorized: true},
		Resolver:      resolver,
	})
	defer cleanup()

	resp := postToolCall(t, addr, `{"mcp":"postgres-ro","tool":"get_user"}`, "Bearer token")
	body := decodeJSONBody(t, resp)

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	assert.Equal(t, "mcp_disabled", body["error"],
		"a disabled MCP must be rejected as mcp_disabled, distinct from mcp_not_found")
}

// toolResultEnvelope is the shape mcp.Client actually produces: the whole
// marshaled CallToolResult, with the payload appearing both as
// structuredContent and, per the MCP spec's own recommendation, serialized
// into a text content block. Fakes that return a bare record test a shape
// the real client cannot emit -- which is how the filtering defect in #159
// survived a green suite.
func toolResultEnvelope(t *testing.T, payload string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"content":           []any{map[string]any{"type": "text", "text": payload}},
		"structuredContent": json.RawMessage(payload),
		"isError":           false,
	})
	require.NoError(t, err)
	return raw
}

func TestToolCall_SuccessfulCallReturnsFilteredBody(t *testing.T) {
	resolver := newFakeResolver()
	resolver.addTool("postgres-ro", "get_user", &fakeMCPClient{
		callTool: func(context.Context, string, map[string]any) (*mcp.ToolResponse, error) {
			raw := toolResultEnvelope(t, `{"id":1,"email":"a@example.com","hashed_password":"secret"}`)
			return &mcp.ToolResponse{Raw: raw}, nil
		},
	})

	addr, cleanup := startTestGatewayWithConfig(t, internal.Config{
		Authenticator: &fakeAuthenticator{claims: validClaims()},
		Policy: &fakePolicyEngine{
			authorized: true,
			dropFields: []string{"$.hashed_password"},
		},
		Resolver: resolver,
	})
	defer cleanup()

	resp := postToolCall(t, addr, `{"mcp":"postgres-ro","tool":"get_user"}`, "Bearer token")
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	raw := readBody(t, resp)
	// Both copies, not just the structured one: the text block carries the
	// same record and leaking it there is the whole of #159.
	assert.NotContains(t, string(raw), "hashed_password")
	assert.NotContains(t, string(raw), "secret")
	assert.Contains(t, string(raw), "a@example.com")
}

// Fail closed: a filter policy applies to this call, but the tool answered
// with free-form text that field paths cannot be applied to. Returning it
// would hand over exactly what the operator asked to strip.
func TestToolCall_UnenforceableFilterFailsClosed(t *testing.T) {
	resolver := newFakeResolver()
	resolver.addTool("postgres-ro", "get_user", &fakeMCPClient{
		callTool: func(context.Context, string, map[string]any) (*mcp.ToolResponse, error) {
			raw := json.RawMessage(`{"content":[{"type":"text","text":"the ssn is 123-45-6789"}]}`)
			return &mcp.ToolResponse{Raw: raw}, nil
		},
	})

	addr, cleanup := startTestGatewayWithConfig(t, internal.Config{
		Authenticator: &fakeAuthenticator{claims: validClaims()},
		Policy: &fakePolicyEngine{
			authorized: true,
			dropFields: []string{"$.ssn"},
		},
		Resolver: resolver,
	})
	defer cleanup()

	resp := postToolCall(t, addr, `{"mcp":"postgres-ro","tool":"get_user"}`, "Bearer token")
	body := decodeJSONBody(t, resp)

	assert.Equal(t, http.StatusBadGateway, resp.StatusCode)
	assert.Equal(t, "filter_unenforceable", body["error"])
	assert.NotContains(t, bodyString(t, body), "123-45-6789", "the unfiltered payload must not be echoed back")
}

// The same response with no filter in force is fine -- there is nothing to
// enforce, so a text-only tool keeps working.
func TestToolCall_FreeFormTextPassesThroughWhenNoFilterApplies(t *testing.T) {
	resolver := newFakeResolver()
	resolver.addTool("postgres-ro", "get_user", &fakeMCPClient{
		callTool: func(context.Context, string, map[string]any) (*mcp.ToolResponse, error) {
			raw := json.RawMessage(`{"content":[{"type":"text","text":"plain prose"}]}`)
			return &mcp.ToolResponse{Raw: raw}, nil
		},
	})

	addr, cleanup := startTestGatewayWithConfig(t, internal.Config{
		Authenticator: &fakeAuthenticator{claims: validClaims()},
		Policy:        &fakePolicyEngine{authorized: true},
		Resolver:      resolver,
	})
	defer cleanup()

	resp := postToolCall(t, addr, `{"mcp":"postgres-ro","tool":"get_user"}`, "Bearer token")

	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, string(readBody(t, resp)), "plain prose")
}

func TestToolCall_DownstreamErrorReturns502(t *testing.T) {
	resolver := newFakeResolver()
	resolver.addTool("postgres-ro", "get_user", &fakeMCPClient{
		callTool: func(context.Context, string, map[string]any) (*mcp.ToolResponse, error) {
			return nil, errors.New("mcp process crashed")
		},
	})

	addr, cleanup := startTestGatewayWithConfig(t, internal.Config{
		Authenticator: &fakeAuthenticator{claims: validClaims()},
		Policy:        &fakePolicyEngine{authorized: true},
		Resolver:      resolver,
	})
	defer cleanup()

	resp := postToolCall(t, addr, `{"mcp":"postgres-ro","tool":"get_user"}`, "Bearer token")
	body := decodeJSONBody(t, resp)

	assert.Equal(t, http.StatusBadGateway, resp.StatusCode)
	assert.Equal(t, "upstream_error", body["error"])
}

// TestToolCall_DownstreamTimeoutReturns504GatewayTimeout uses a very short
// CallTimeout and a fake client that blocks past it, proving the request
// does not hang past its configured deadline and gets a distinct status
// from a plain downstream error.
func TestToolCall_DownstreamTimeoutReturns504GatewayTimeout(t *testing.T) {
	resolver := newFakeResolver()
	resolver.addTool("postgres-ro", "get_user", &fakeMCPClient{
		callTool: func(ctx context.Context, _ string, _ map[string]any) (*mcp.ToolResponse, error) {
			<-ctx.Done() // block until the gateway's call-scoped context expires
			return nil, ctx.Err()
		},
	})

	addr, cleanup := startTestGatewayWithConfig(t, internal.Config{
		Authenticator: &fakeAuthenticator{claims: validClaims()},
		Policy:        &fakePolicyEngine{authorized: true},
		Resolver:      resolver,
		CallTimeout:   200 * time.Millisecond,
	})
	defer cleanup()

	start := time.Now()
	resp := postToolCall(t, addr, `{"mcp":"postgres-ro","tool":"get_user"}`, "Bearer token")
	elapsed := time.Since(start)
	body := decodeJSONBody(t, resp)

	assert.Equal(t, http.StatusGatewayTimeout, resp.StatusCode)
	assert.Equal(t, "upstream_timeout", body["error"])
	assert.Less(t, elapsed, 4*time.Second, "the request must not hang well past its configured CallTimeout")
}

// readBody drains the response body for assertions that need the raw bytes
// rather than a decoded map.
func readBody(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return raw
}

func bodyString(t *testing.T, body map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	return string(raw)
}
