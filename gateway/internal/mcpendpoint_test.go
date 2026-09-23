package internal_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/atsokha/mcplake/cache"
	"github.com/atsokha/mcplake/internal"
	"github.com/atsokha/mcplake/mcp"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bearerRoundTripper attaches a bearer token to every request an MCP client
// transport makes. This is how a real client authenticates to the gateway:
// the sdk's transports take an *http.Client, not a header list.
type bearerRoundTripper struct {
	token string
	base  http.RoundTripper
}

func (b *bearerRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	if b.token != "" {
		clone.Header.Set("Authorization", "Bearer "+b.token)
	}
	base := b.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(clone)
}

func bearerClient(token string) *http.Client {
	return &http.Client{Transport: &bearerRoundTripper{token: token}}
}

// mcpEndpointFixture starts a gateway with one registered MCP whose single
// tool echoes a filtered payload back, and returns its base URL.
func mcpEndpointFixture(t *testing.T, policy internal.PolicyEngine) string {
	t.Helper()

	resolver := newFakeResolver()
	resolver.addToolSchema("postgres-ro", cache.ToolSchema{
		Name:        "get_user",
		Description: "look up a user by id",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"integer"}}}`),
	})
	resolver.clients["postgres-ro"] = &fakeMCPClient{
		callTool: func(_ context.Context, _ string, _ map[string]any) (*mcp.ToolResponse, error) {
			return &mcp.ToolResponse{
				Raw: toolResultEnvelope(t, `{"id":1,"email":"a@example.com","hashed_password":"secret"}`),
			}, nil
		},
	}

	addr, cleanup := startTestGatewayWithConfig(t, internal.Config{
		Authenticator: &fakeAuthenticator{claims: validClaims()},
		Policy:        policy,
		Resolver:      resolver,
	})
	t.Cleanup(cleanup)
	return "http://" + addr
}

// mcpTransports is the pair this ticket exists for. Every behavioural test
// below runs over both, because "works on streamable" is exactly the gap
// that would go unnoticed: the two transports differ in how they carry
// headers, how sessions are established, and which context a session holds.
func mcpTransports(base string, client *http.Client) map[string]sdk.Transport {
	return map[string]sdk.Transport{
		"streamable": &sdk.StreamableClientTransport{
			Endpoint:   base + internal.MCPStreamablePath,
			HTTPClient: client,
		},
		"sse": &sdk.SSEClientTransport{
			Endpoint:   base + internal.MCPSSEPath,
			HTTPClient: client,
		},
	}
}

func connectMCP(t *testing.T, transport sdk.Transport) *sdk.ClientSession {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	client := sdk.NewClient(&sdk.Implementation{Name: "mcplake-endpoint-test", Version: "0.1.0"}, nil)
	session, err := client.Connect(ctx, transport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func TestMCPEndpoint_ListsAndCallsToolsOverBothTransports(t *testing.T) {
	base := mcpEndpointFixture(t, &fakePolicyEngine{
		authorized: true,
		dropFields: []string{"$.hashed_password"},
	})

	for name, transport := range mcpTransports(base, bearerClient("valid-token")) {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			session := connectMCP(t, transport)

			listed, err := session.ListTools(ctx, nil)
			require.NoError(t, err)
			require.Len(t, listed.Tools, 1)
			assert.Equal(t, "postgres-ro__get_user", listed.Tools[0].Name)
			assert.Equal(t, "look up a user by id", listed.Tools[0].Description)
			assert.NotNil(t, listed.Tools[0].InputSchema, "a client needs the input schema to build a call")

			result, err := session.CallTool(ctx, &sdk.CallToolParams{
				Name:      "postgres-ro__get_user",
				Arguments: map[string]any{"id": 1},
			})
			require.NoError(t, err)
			require.False(t, result.IsError)

			encoded, err := json.Marshal(result)
			require.NoError(t, err)
			assert.Contains(t, string(encoded), "a@example.com")
			assert.NotContains(t, string(encoded), "secret",
				"drop_fields must be applied on the MCP surface exactly as on POST /v1/call")
		})
	}
}

// The catalogue is the access policy made visible. Two callers with
// different grants must see different tool lists over either transport.
func TestMCPEndpoint_ToolListReflectsTheCallersGrants(t *testing.T) {
	base := mcpEndpointFixture(t, &fakePolicyEngine{authorized: false})

	for name, transport := range mcpTransports(base, bearerClient("valid-token")) {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			session := connectMCP(t, transport)

			listed, err := session.ListTools(ctx, nil)
			require.NoError(t, err)
			assert.Empty(t, listed.Tools, "an ungranted caller must see an empty catalogue, not a 403 on discovery")
		})
	}
}

func TestMCPEndpoint_RejectsAnUnauthenticatedSession(t *testing.T) {
	base := mcpEndpointFixture(t, &fakePolicyEngine{authorized: true})

	for name, transport := range mcpTransports(base, bearerClient("")) {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			client := sdk.NewClient(&sdk.Implementation{Name: "mcplake-endpoint-test", Version: "0.1.0"}, nil)
			session, err := client.Connect(ctx, transport, nil)
			if session != nil {
				_ = session.Close()
			}
			require.Error(t, err, "a session must not be established without a bearer token")
		})
	}
}

func TestMCPEndpoint_RejectsAnInvalidToken(t *testing.T) {
	resolver := newFakeResolver()
	addr, cleanup := startTestGatewayWithConfig(t, internal.Config{
		Authenticator: &fakeAuthenticator{err: fmt.Errorf("expired")},
		Policy:        &fakePolicyEngine{authorized: true},
		Resolver:      resolver,
	})
	t.Cleanup(cleanup)

	for name, transport := range mcpTransports("http://"+addr, bearerClient("expired-token")) {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			client := sdk.NewClient(&sdk.Implementation{Name: "mcplake-endpoint-test", Version: "0.1.0"}, nil)
			session, err := client.Connect(ctx, transport, nil)
			if session != nil {
				_ = session.Close()
			}
			require.Error(t, err)
		})
	}
}

// POST /v1/call is not replaced by any of this, and the two surfaces have to
// keep answering the same way.
func TestMCPEndpoint_DoesNotDisturbTheRESTToolCallPath(t *testing.T) {
	base := mcpEndpointFixture(t, &fakePolicyEngine{
		authorized: true,
		dropFields: []string{"$.hashed_password"},
	})

	body, err := json.Marshal(map[string]any{"mcp": "postgres-ro", "tool": "get_user"})
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, base+"/v1/call", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer valid-token")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// The gateway must advertise the tools capability even though it registers
// no tool with AddTool: its catalogue is resolved per request, so nothing is
// there for the sdk to infer the capability from.
func TestMCPEndpoint_AdvertisesTheToolsCapability(t *testing.T) {
	base := mcpEndpointFixture(t, &fakePolicyEngine{authorized: true})

	for name, transport := range mcpTransports(base, bearerClient("valid-token")) {
		t.Run(name, func(t *testing.T) {
			session := connectMCP(t, transport)
			require.NotNil(t, session.InitializeResult().Capabilities.Tools,
				"a client that sees no tools capability will never call tools/list")
		})
	}
}

// An MCP session holds a hanging GET open for its whole life, and fasthttp's
// graceful shutdown waits for in-flight requests. If the gateway does not
// end its sessions *before* asking the server to shut down, Stop blocks
// until its deadline -- which is a hung process on SIGTERM, and a hung suite
// in CI.
func TestMCPEndpoint_StopReturnsWithSessionsStillOpen(t *testing.T) {
	resolver := newFakeResolver()
	resolver.addToolSchema("postgres-ro", cache.ToolSchema{Name: "get_user"})

	g := internal.NewGateway(internal.Config{
		DataPlaneAddr: "127.0.0.1:0",
		Authenticator: &fakeAuthenticator{claims: validClaims()},
		Policy:        &fakePolicyEngine{authorized: true},
		Resolver:      resolver,
	})
	startErr := make(chan error, 1)
	go func() { startErr <- g.Start(context.Background()) }()

	select {
	case <-g.Ready():
	case err := <-startErr:
		t.Fatalf("gateway failed to start: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for gateway readiness")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Connect over both transports and deliberately leave both open.
	for _, transport := range mcpTransports("http://"+g.Addr(), bearerClient("valid-token")) {
		client := sdk.NewClient(&sdk.Implementation{Name: "mcplake-endpoint-test", Version: "0.1.0"}, nil)
		_, err := client.Connect(ctx, transport, nil)
		require.NoError(t, err)
	}

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	require.NoError(t, g.Stop(stopCtx), "Stop must not block on open MCP sessions")

	select {
	case err := <-startErr:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after Stop")
	}
}

// The SSE transport's message POSTs may omit Authorization, because the sdk's
// own SSE client does not send it there. That exemption keys off a
// `?sessionid=` query parameter — and must not leak onto the streamable
// endpoint, whose sessions are keyed by an Mcp-Session-Id *header*. If it
// did, appending any `?sessionid=` would skip token validation outright.
func TestMCPEndpoint_StreamableDoesNotHonourTheSSESessionIDExemption(t *testing.T) {
	base := mcpEndpointFixture(t, &fakePolicyEngine{authorized: true})

	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	req, err := http.NewRequest(http.MethodPost, base+internal.MCPStreamablePath+"?sessionid=anything", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Session-Id", "smuggled")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"a streamable request without a bearer token must be refused whatever its query string says")
}
