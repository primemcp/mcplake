package internal_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/primemcp/mcplake/auth"
	"github.com/primemcp/mcplake/internal"
	"github.com/primemcp/mcplake/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func postToolCall(t *testing.T, addr, body, authHeader string) *http.Response {
	t.Helper()
	return doToolCall(t, http.DefaultClient, addr, body, authHeader)
}

func doToolCall(t *testing.T, client *http.Client, addr, body, authHeader string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s/v1/call", addr), strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	resp, err := client.Do(req)
	require.NoError(t, err)
	return resp
}

func decodeJSONBody(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	return body
}

func TestToolCall_MalformedJSONReturns400(t *testing.T) {
	addr, cleanup := startTestGateway(t)
	defer cleanup()

	resp := postToolCall(t, addr, `{not-json`, "Bearer abc")
	body := decodeJSONBody(t, resp)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "invalid_json", body["error"])
}

func TestToolCall_MissingMCPReturns400(t *testing.T) {
	addr, cleanup := startTestGateway(t)
	defer cleanup()

	resp := postToolCall(t, addr, `{"tool":"get_user"}`, "Bearer abc")
	body := decodeJSONBody(t, resp)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "missing_field", body["error"])
}

func TestToolCall_MissingToolReturns400(t *testing.T) {
	addr, cleanup := startTestGateway(t)
	defer cleanup()

	resp := postToolCall(t, addr, `{"mcp":"postgres-ro"}`, "Bearer abc")
	body := decodeJSONBody(t, resp)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "missing_field", body["error"])
}

func TestToolCall_MissingAuthorizationReturns401(t *testing.T) {
	addr, cleanup := startTestGateway(t)
	defer cleanup()

	resp := postToolCall(t, addr, `{"mcp":"postgres-ro","tool":"get_user"}`, "")
	body := decodeJSONBody(t, resp)

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.Equal(t, "missing_authorization", body["error"])
}

func TestToolCall_WrongAuthSchemeReturns401(t *testing.T) {
	addr, cleanup := startTestGateway(t)
	defer cleanup()

	resp := postToolCall(t, addr, `{"mcp":"postgres-ro","tool":"get_user"}`, "Basic dXNlcjpwYXNz")
	body := decodeJSONBody(t, resp)

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.Equal(t, "invalid_authorization", body["error"])
}

func TestToolCall_PipelineNotConfiguredReturns501(t *testing.T) {
	// startTestGateway wires no Authenticator/Policy/Resolver - the pipeline
	// must fail closed with 501, not nil-panic.
	addr, cleanup := startTestGateway(t)
	defer cleanup()

	resp := postToolCall(t, addr, `{"mcp":"postgres-ro","tool":"get_user","arguments":{}}`, "Bearer abc.def.ghi")
	body := decodeJSONBody(t, resp)

	assert.Equal(t, http.StatusNotImplemented, resp.StatusCode)
	assert.Equal(t, "not_implemented", body["error"])
}

// TestToolCall_ConcurrentRequestsDoNotCrossContaminate fires many concurrent
// requests, each routed to its own distinct (mcp, tool), and asserts every
// response reflects exactly its own request's tool, not another concurrent
// request's. fasthttp pools and reuses RequestCtx across connections; if
// parseToolCallRequest ever held a reference into a pooled buffer instead of
// a copy, concurrent requests under load would intermittently be routed to
// the wrong MCP/tool or see another request's response.
func TestToolCall_ConcurrentRequestsDoNotCrossContaminate(t *testing.T) {
	const n = 200
	resolver := newFakeResolver()
	for i := range n {
		mcpName := fmt.Sprintf("mcp-%d", i)
		toolName := fmt.Sprintf("tool-%d", i)
		resolver.addTool(mcpName, toolName, &fakeMCPClient{
			callTool: func(_ context.Context, tool string, _ map[string]any) (*mcp.ToolResponse, error) {
				raw, err := json.Marshal(map[string]string{"echo_tool": tool})
				require.NoError(t, err)
				return &mcp.ToolResponse{Raw: raw}, nil
			},
		})
	}

	addr, cleanup := startTestGatewayWithConfig(t, internal.Config{
		Authenticator: &fakeAuthenticator{claims: &auth.Claims{Raw: json.RawMessage(`{}`)}},
		Policy:        &fakePolicyEngine{authorized: true},
		Resolver:      resolver,
	})
	defer cleanup()

	// Keep-alive connections deliberately are not force-closed by
	// Gateway.Stop (see gateway.go's idleTimeout comment); using
	// non-persistent connections here means this test's cleanup doesn't
	// depend on the server's idle timeout, which is tuned for production,
	// not test speed.
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}

	var wg sync.WaitGroup
	wg.Add(n)
	for i := range n {
		go func(i int) {
			defer wg.Done()
			mcpName := fmt.Sprintf("mcp-%d", i)
			toolName := fmt.Sprintf("tool-%d", i)
			body := fmt.Sprintf(`{"mcp":%q,"tool":%q}`, mcpName, toolName)

			resp := doToolCall(t, client, addr, body, "Bearer token")
			got := decodeJSONBody(t, resp)

			assert.Equal(t, http.StatusOK, resp.StatusCode)
			assert.Equal(t, toolName, got["echo_tool"], "response must reflect this request's own tool, not another concurrent request's")
		}(i)
	}
	wg.Wait()
}
