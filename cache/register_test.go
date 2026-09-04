package cache

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/atsokha/mcplake/mcp"
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
}

func (f *fakeMCPClient) ListTools(context.Context) ([]mcp.ToolSchema, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.tools, nil
}

func (f *fakeMCPClient) CallTool(context.Context, string, map[string]any) (*mcp.ToolResponse, error) {
	return nil, errors.New("not used in these tests")
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
