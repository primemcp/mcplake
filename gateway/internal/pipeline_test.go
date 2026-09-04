package internal_test

import (
	"context"
	"encoding/json"
	"errors"
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

func TestToolCall_SuccessfulCallReturnsFilteredBody(t *testing.T) {
	resolver := newFakeResolver()
	resolver.addTool("postgres-ro", "get_user", &fakeMCPClient{
		callTool: func(context.Context, string, map[string]any) (*mcp.ToolResponse, error) {
			raw := json.RawMessage(`{"id":1,"email":"a@example.com","hashed_password":"secret"}`)
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
	body := decodeJSONBody(t, resp)
	assert.Equal(t, "a@example.com", body["email"])
	assert.NotContains(t, body, "hashed_password", "the field filter's DropFields must have been applied")
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
