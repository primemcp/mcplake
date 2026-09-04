package internal_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/atsokha/mcplake/auth"
	"github.com/atsokha/mcplake/internal"
	"github.com/atsokha/mcplake/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestToolCall_LoadWithArtificialLatencyForcesRequestCtxReuse is ticket
// #10's dedicated regression test: it deliberately uses keep-alive
// connections with a small connection pool (so a handful of real TCP
// connections carry hundreds of sequential requests) and an artificial
// per-call sleep in the fake MCP client, holding each handler invocation
// open long enough that fasthttp is very likely to reuse a connection's
// pooled fasthttp.RequestCtx for a different logical request while an
// earlier one on that connection is still executing. If parseToolCallRequest
// (ticket #8) or runPipeline (ticket #9) ever captured a RequestCtx-backed
// byte slice/string instead of a copy, this is the scenario that would
// surface it - as a response echoing the wrong tool, and/or as a data race
// caught by go test -race.
func TestToolCall_LoadWithArtificialLatencyForcesRequestCtxReuse(t *testing.T) {
	const n = 300
	const artificialLatency = 25 * time.Millisecond

	resolver := newFakeResolver()
	for i := range n {
		mcpName := fmt.Sprintf("mcp-%d", i)
		toolName := fmt.Sprintf("tool-%d", i)
		resolver.addTool(mcpName, toolName, &fakeMCPClient{
			callTool: func(_ context.Context, tool string, _ map[string]any) (*mcp.ToolResponse, error) {
				time.Sleep(artificialLatency)
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

	// A small connection pool with keep-alives enabled (unlike the
	// DisableKeepAlives client used elsewhere) is what actually forces one
	// connection's RequestCtx to be reused across many different logical
	// requests in sequence, rather than each request getting a fresh one.
	transport := &http.Transport{MaxConnsPerHost: 8}
	client := &http.Client{Transport: transport}

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
			assert.Equal(t, toolName, got["echo_tool"],
				"response must reflect this request's own tool despite heavy connection/RequestCtx reuse under load")
		}(i)
	}
	wg.Wait()

	// Close idle keep-alive connections now, from the test, rather than
	// leaving it to cleanup()'s Stop: fasthttp's ShutdownWithContext
	// deliberately does not force-close keepalive connections itself (see
	// gateway.go's idleTimeout comment), and cleanup uses a short Stop
	// deadline tuned for fast test teardown, not for waiting out this
	// client's connection pool.
	transport.CloseIdleConnections()
}
