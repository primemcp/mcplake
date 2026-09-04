package internal_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

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

func TestToolCall_ValidRequestReturns501Stub(t *testing.T) {
	addr, cleanup := startTestGateway(t)
	defer cleanup()

	resp := postToolCall(t, addr, `{"mcp":"postgres-ro","tool":"get_user","arguments":{}}`, "Bearer abc.def.ghi")
	body := decodeJSONBody(t, resp)

	assert.Equal(t, http.StatusNotImplemented, resp.StatusCode)
	assert.Equal(t, "not_implemented", body["error"])
	assert.Equal(t, "postgres-ro", body["mcp"])
	assert.Equal(t, "get_user", body["tool"])
}

// TestToolCall_ConcurrentRequestsDoNotCrossContaminate fires many concurrent
// requests, each with a distinct mcp/tool payload, and asserts every response
// echoes back exactly its own request's values. fasthttp pools and reuses
// RequestCtx across connections; if parseToolCallRequest ever held a
// reference into a pooled buffer instead of a copy, concurrent requests under
// load would intermittently observe another request's mcp/tool in the echoed
// response.
func TestToolCall_ConcurrentRequestsDoNotCrossContaminate(t *testing.T) {
	addr, cleanup := startTestGateway(t)
	defer cleanup()

	// Keep-alive connections deliberately are not force-closed by
	// Gateway.Stop (see gateway.go's idleTimeout comment); using
	// non-persistent connections here means this test's cleanup doesn't
	// depend on the server's idle timeout, which is tuned for production,
	// not test speed.
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}

	const n = 200
	var wg sync.WaitGroup
	wg.Add(n)
	for i := range n {
		go func(i int) {
			defer wg.Done()
			mcp := fmt.Sprintf("mcp-%d", i)
			tool := fmt.Sprintf("tool-%d", i)
			body := fmt.Sprintf(`{"mcp":%q,"tool":%q}`, mcp, tool)

			resp := doToolCall(t, client, addr, body, "Bearer token")
			got := decodeJSONBody(t, resp)

			assert.Equal(t, http.StatusNotImplemented, resp.StatusCode)
			assert.Equal(t, mcp, got["mcp"], "response mcp must match this request's own mcp, not another concurrent request's")
			assert.Equal(t, tool, got["tool"], "response tool must match this request's own tool, not another concurrent request's")
		}(i)
	}
	wg.Wait()
}
