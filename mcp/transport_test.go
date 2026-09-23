package mcp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/atsokha/mcplake/mcp"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixtureServer builds the same one-tool server the stdio fixture process
// runs, for mounting behind an HTTP handler instead of a pipe. Keeping the
// tool identical is the point: the assertions below are then about the
// transport and nothing else.
func fixtureServer() *sdk.Server {
	server := sdk.NewServer(&sdk.Implementation{Name: "mcplake-fixture", Version: "0.1.0"}, nil)
	sdk.AddTool(server, &sdk.Tool{
		Name:        "echo",
		Description: "echoes the given message back",
	}, func(_ context.Context, _ *sdk.CallToolRequest, args echoArgs) (*sdk.CallToolResult, echoResult, error) {
		return nil, echoResult{Message: args.Message}, nil
	})
	return server
}

// httptest binds 127.0.0.1, so every URL below is loopback and passes
// ValidateEndpointURL without the test having to weaken anything.
func newHTTPFixture(t *testing.T) string {
	t.Helper()
	handler := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return fixtureServer() }, nil)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv.URL
}

func newSSEFixture(t *testing.T) string {
	t.Helper()
	handler := sdk.NewSSEHandler(func(*http.Request) *sdk.Server { return fixtureServer() }, nil)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv.URL
}

// The whole point of ADR-0017: an MCP the gateway does not start, reached
// over the network, behaving exactly like one it spawned.
func TestNewClient_HTTPTransportListsAndCallsTools(t *testing.T) {
	ctx := context.Background()
	client, err := mcp.NewClient(ctx, mcp.Config{
		Transport: mcp.TransportHTTP,
		URL:       newHTTPFixture(t),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	tools, err := client.ListTools(ctx)
	require.NoError(t, err)
	require.Len(t, tools, 1)
	assert.Equal(t, "echo", tools[0].Name)

	resp, err := client.CallTool(ctx, "echo", map[string]any{"message": "over http"})
	require.NoError(t, err)
	assert.False(t, resp.IsError)
	assert.Contains(t, string(resp.Raw), "over http")
}

func TestNewClient_SSETransportListsAndCallsTools(t *testing.T) {
	ctx := context.Background()
	client, err := mcp.NewClient(ctx, mcp.Config{
		Transport: mcp.TransportSSE,
		URL:       newSSEFixture(t),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	tools, err := client.ListTools(ctx)
	require.NoError(t, err)
	require.Len(t, tools, 1)
	assert.Equal(t, "echo", tools[0].Name)

	resp, err := client.CallTool(ctx, "echo", map[string]any{"message": "over sse"})
	require.NoError(t, err)
	assert.False(t, resp.IsError)
	assert.Contains(t, string(resp.Raw), "over sse")
}

// Every caller written before there was more than one transport passed a
// Command and no Transport at all, and must keep working untouched.
func TestNewClient_EmptyTransportStillMeansStdio(t *testing.T) {
	_, err := mcp.NewClient(context.Background(), mcp.Config{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Config.Command is required")
	assert.Contains(t, err.Error(), "stdio", "the error should name the transport it defaulted to")
}

func TestNewClient_RejectsIncompleteConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     mcp.Config
		wantErr string
	}{
		{
			name:    "http without a url",
			cfg:     mcp.Config{Transport: mcp.TransportHTTP},
			wantErr: "Config.URL is required",
		},
		{
			name:    "sse without a url",
			cfg:     mcp.Config{Transport: mcp.TransportSSE},
			wantErr: "Config.URL is required",
		},
		{
			name:    "stdio without a command",
			cfg:     mcp.Config{Transport: mcp.TransportStdio},
			wantErr: "Config.Command is required",
		},
		{
			name:    "a transport that does not exist",
			cfg:     mcp.Config{Transport: "carrier-pigeon", URL: "https://mcp.example.com/mcp"},
			wantErr: `unsupported transport "carrier-pigeon"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := mcp.NewClient(context.Background(), tt.cfg)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// A tool call's arguments and its response are the payloads the whole
// gateway exists to control access to. Shipping them to a remote host in
// the clear has to fail before anything is dialed, not at first call.
func TestNewClient_RefusesPlaintextToARemoteHost(t *testing.T) {
	_, err := mcp.NewClient(context.Background(), mcp.Config{
		Transport: mcp.TransportHTTP,
		URL:       "http://mcp.example.com/mcp",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must use https")
}

func TestValidateEndpointURL(t *testing.T) {
	valid := []string{
		"https://mcp.example.com/mcp",
		"https://mcp.example.com:8443/mcp",
		"http://localhost:8931/mcp",
		"http://LOCALHOST:8931/mcp",
		"http://127.0.0.1:8931/mcp",
		"http://[::1]:8931/mcp",
	}
	for _, raw := range valid {
		t.Run("valid/"+raw, func(t *testing.T) {
			assert.NoError(t, mcp.ValidateEndpointURL(raw))
		})
	}

	invalid := map[string]string{
		"http://mcp.example.com/mcp": "must use https",
		"http://10.0.0.5:8931/mcp":   "must use https",
		"ftp://mcp.example.com/mcp":  "must be an absolute http(s) URL",
		"/mcp":                       "must be an absolute http(s) URL",
		"https://":                   "must include a host",
		"":                           "must be an absolute http(s) URL",
	}
	for raw, wantErr := range invalid {
		t.Run("invalid/"+raw, func(t *testing.T) {
			err := mcp.ValidateEndpointURL(raw)
			require.Error(t, err)
			assert.Contains(t, err.Error(), wantErr)
		})
	}
}

func TestSupportedTransports(t *testing.T) {
	assert.Equal(t, []string{"stdio", "http", "sse"}, mcp.SupportedTransports())
	for _, tr := range mcp.SupportedTransports() {
		assert.True(t, mcp.TransportSupported(tr), tr)
	}
	assert.True(t, mcp.TransportSupported(""), "an omitted transport means stdio")
	assert.False(t, mcp.TransportSupported("grpc"))
}
