package cache

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/primemcp/mcplake/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// White-box (package cache) so tests can override the newMCPClient seam
// without spawning a real subprocess.

// fakeMCPClient is a minimal MCPClient test double.
type fakeMCPClient struct {
	tools    []mcp.ToolSchema
	listErr  error
	closed   bool
	closeErr error
	// onListTools, if set, runs at the start of each ListTools call — used
	// by the refresh tests to simulate a registration swap racing the
	// discovery call.
	onListTools func()
	// pingErr is what Ping returns; nil means a healthy session. The health
	// tests set it to simulate a downstream that went away underneath a
	// live registration.
	pingErr error
	// pingFailures, when > 0, makes only that many leading Ping calls fail
	// (with pingErr, or a generic error) and the rest succeed — the shape of
	// a stale pooled connection, where the socket is dead but the session
	// behind it is not.
	pingFailures int
	// pings counts Ping calls, so a test can assert a health check probed
	// (or deliberately did not probe) a given client.
	pings int
}

func (f *fakeMCPClient) ListTools(context.Context) ([]mcp.ToolSchema, error) {
	if f.onListTools != nil {
		f.onListTools()
	}
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.tools, nil
}

func (f *fakeMCPClient) CallTool(context.Context, string, map[string]any) (*mcp.ToolResponse, error) {
	return nil, errors.New("not used in these tests")
}

func (f *fakeMCPClient) Ping(context.Context) error {
	f.pings++
	if f.pingFailures > 0 {
		f.pingFailures--
		if f.pingErr != nil {
			return f.pingErr
		}
		return errors.New("transient ping failure")
	}
	return f.pingErr
}

func (f *fakeMCPClient) Close() error {
	f.closed = true
	return f.closeErr
}

// withFakeNewMCPClient overrides newMCPClient for the duration of a test.
func withFakeNewMCPClient(t *testing.T, fn func(ctx context.Context, cfg mcp.Config) (MCPClient, error)) {
	t.Helper()
	original := newMCPClient
	newMCPClient = fn
	t.Cleanup(func() { newMCPClient = original })
}

func TestRegistry_Register_Success(t *testing.T) {
	fake := &fakeMCPClient{
		tools: []mcp.ToolSchema{
			{Name: "get_user", InputSchema: json.RawMessage(`{"type":"object"}`)},
		},
	}
	withFakeNewMCPClient(t, func(context.Context, mcp.Config) (MCPClient, error) {
		return fake, nil
	})

	r := NewRegistry()
	err := r.Register(context.Background(), MCPRegistration{
		Name:      "postgres-ro",
		Transport: "stdio",
		Connect:   ConnectConfig{Command: "mcp-server-postgres"},
	})

	require.NoError(t, err)

	got, ok := r.Get("postgres-ro")
	require.True(t, ok)
	assert.Equal(t, StatusActive, got.Status)
	assert.True(t, r.HasTool("postgres-ro", "get_user"))
	assert.Same(t, fake, got.Client)
	assert.False(t, fake.closed, "a successfully registered client must not be closed")
}

func TestRegistry_Register_ConnectionFailureStoresUnreachable(t *testing.T) {
	withFakeNewMCPClient(t, func(context.Context, mcp.Config) (MCPClient, error) {
		return nil, errors.New("connection refused")
	})

	r := NewRegistry()
	err := r.Register(context.Background(), MCPRegistration{
		Name:      "postgres-ro",
		Transport: "stdio",
		Connect:   ConnectConfig{Command: "mcp-server-postgres"},
	})

	require.Error(t, err)

	got, ok := r.Get("postgres-ro")
	require.True(t, ok, "a failed registration must still be recorded, as unreachable")
	assert.Equal(t, StatusUnreachable, got.Status)
	assert.Empty(t, got.Tools)
	assert.Nil(t, got.Client)
}

func TestRegistry_Register_ToolDiscoveryFailureStoresUnreachableAndClosesClient(t *testing.T) {
	fake := &fakeMCPClient{listErr: errors.New("tools/list failed")}
	withFakeNewMCPClient(t, func(context.Context, mcp.Config) (MCPClient, error) {
		return fake, nil
	})

	r := NewRegistry()
	err := r.Register(context.Background(), MCPRegistration{
		Name:      "postgres-ro",
		Transport: "stdio",
		Connect:   ConnectConfig{Command: "mcp-server-postgres"},
	})

	require.Error(t, err)

	got, ok := r.Get("postgres-ro")
	require.True(t, ok)
	assert.Equal(t, StatusUnreachable, got.Status)
	assert.True(t, fake.closed, "the client must be closed when discovery fails, to avoid leaking the subprocess")
}

func TestRegistry_Register_RequiresName(t *testing.T) {
	r := NewRegistry()
	err := r.Register(context.Background(), MCPRegistration{Transport: "stdio"})
	assert.Error(t, err)
}

func TestRegistry_Register_UnsupportedTransportStoresUnreachable(t *testing.T) {
	r := NewRegistry()
	err := r.Register(context.Background(), MCPRegistration{
		Name:      "some-sse-mcp",
		Transport: "sse",
	})

	require.Error(t, err)
	got, ok := r.Get("some-sse-mcp")
	require.True(t, ok)
	assert.Equal(t, StatusUnreachable, got.Status)
}

func TestRegistry_Register_OneFailureDoesNotAffectOtherRegistrations(t *testing.T) {
	goodClient := &fakeMCPClient{tools: []mcp.ToolSchema{{Name: "get_user"}}}
	calls := 0
	withFakeNewMCPClient(t, func(context.Context, mcp.Config) (MCPClient, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("first MCP unreachable")
		}
		return goodClient, nil
	})

	r := NewRegistry()
	err1 := r.Register(context.Background(), MCPRegistration{Name: "bad-mcp", Transport: "stdio", Connect: ConnectConfig{Command: "x"}})
	err2 := r.Register(context.Background(), MCPRegistration{Name: "good-mcp", Transport: "stdio", Connect: ConnectConfig{Command: "y"}})

	assert.Error(t, err1)
	assert.NoError(t, err2)

	bad, _ := r.Get("bad-mcp")
	good, _ := r.Get("good-mcp")
	assert.Equal(t, StatusUnreachable, bad.Status)
	assert.Equal(t, StatusActive, good.Status)
}

// ADR-0017: an MCP the gateway does not start. The Registry's own behaviour
// must not vary by transport -- same Active status, same discovered tools,
// same Connect round-trip -- so these assert the exact shape the stdio tests
// above assert, with a URL in place of a command.
func TestRegister_AcceptsHTTPAndSSETransports(t *testing.T) {
	for _, transport := range []string{mcp.TransportHTTP, mcp.TransportSSE} {
		t.Run(transport, func(t *testing.T) {
			var got mcp.Config
			withFakeNewMCPClient(t, func(_ context.Context, cfg mcp.Config) (MCPClient, error) {
				got = cfg
				return &fakeMCPClient{tools: []mcp.ToolSchema{{Name: "echo"}}}, nil
			})

			r := NewRegistry()
			reg := MCPRegistration{
				Name:      "remote",
				Transport: transport,
				Connect:   ConnectConfig{URL: "https://mcp.example.com/mcp"},
				Enabled:   true,
			}
			require.NoError(t, r.Register(context.Background(), reg))

			// The transport and URL must reach mcp.NewClient; before
			// ADR-0017 Register built a Config with only Command/Arguments,
			// so a URL would have been silently dropped.
			assert.Equal(t, transport, got.Transport)
			assert.Equal(t, "https://mcp.example.com/mcp", got.URL)

			stored, ok := r.Get("remote")
			require.True(t, ok)
			assert.Equal(t, StatusActive, stored.Status)
			assert.Equal(t, transport, stored.Transport)
			assert.Equal(t, "https://mcp.example.com/mcp", stored.Connect.URL)
			assert.Len(t, stored.Tools, 1)
		})
	}
}

func TestRegister_RejectsAnUnknownTransport(t *testing.T) {
	r := NewRegistry()
	err := r.Register(context.Background(), MCPRegistration{
		Name:      "pigeon",
		Transport: "carrier-pigeon",
		Connect:   ConnectConfig{URL: "https://mcp.example.com/mcp"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unsupported transport "carrier-pigeon"`)
	assert.Contains(t, err.Error(), "stdio, http, sse", "the error should list what is supported")

	// Still recorded, so an operator can see the failed registration rather
	// than wondering where their entry went -- same as any other
	// first-attempt failure.
	stored, ok := r.Get("pigeon")
	require.True(t, ok)
	assert.Equal(t, StatusUnreachable, stored.Status)
}

// An omitted transport has always meant stdio. It must keep meaning that,
// and must be settled to the concrete name before anything stores it, so no
// API response or database row ever carries an empty transport.
func TestRegister_NormalizesAnOmittedTransportToStdio(t *testing.T) {
	withFakeNewMCPClient(t, func(context.Context, mcp.Config) (MCPClient, error) {
		return &fakeMCPClient{}, nil
	})

	r := NewRegistry()
	require.NoError(t, r.Register(context.Background(), MCPRegistration{
		Name:    "legacy",
		Connect: ConnectConfig{Command: "some-server"},
	}))

	stored, ok := r.Get("legacy")
	require.True(t, ok)
	assert.Equal(t, mcp.TransportStdio, stored.Transport)
}

// TestRegistry_Register_KeepsToolDescriptions pins the field the data-plane
// MCP endpoint needs: an MCP client picks a tool by its description, so
// discovery has to store it rather than drop it. See ADR-0021.
func TestRegistry_Register_KeepsToolDescriptions(t *testing.T) {
	fake := &fakeMCPClient{
		tools: []mcp.ToolSchema{{
			Name:        "get_user",
			Description: "look up a user by id",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		}},
	}
	withFakeNewMCPClient(t, func(context.Context, mcp.Config) (MCPClient, error) {
		return fake, nil
	})

	r := NewRegistry()
	require.NoError(t, r.Register(context.Background(), MCPRegistration{
		Name:      "postgres-ro",
		Transport: "stdio",
		Connect:   ConnectConfig{Command: "mcp-server-postgres"},
	}))

	got, ok := r.Get("postgres-ro")
	require.True(t, ok)
	assert.Equal(t, "look up a user by id", got.Tools["get_user"].Description)
}
