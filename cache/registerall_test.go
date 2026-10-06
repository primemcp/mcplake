package cache

import (
	"context"
	"errors"
	"testing"

	"github.com/primemcp/mcplake/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry_RegisterAll_AllReachableBecomeActive(t *testing.T) {
	withFakeNewMCPClient(t, func(context.Context, mcp.Config) (MCPClient, error) {
		return &fakeMCPClient{tools: []mcp.ToolSchema{{Name: "get_user"}}}, nil
	})

	r := NewRegistry()
	r.RegisterAll(context.Background(), []MCPRegistration{
		{Name: "postgres-ro", Transport: "stdio", Connect: ConnectConfig{Command: "a"}},
		{Name: "postgres-rw", Transport: "stdio", Connect: ConnectConfig{Command: "b"}},
		{Name: "filesystem", Transport: "stdio", Connect: ConnectConfig{Command: "c"}},
	})

	for _, name := range []string{"postgres-ro", "postgres-rw", "filesystem"} {
		got, ok := r.Get(name)
		require.True(t, ok, "expected %q to be registered", name)
		assert.Equal(t, StatusActive, got.Status)
	}
}

func TestRegistry_RegisterAll_OneUnreachableDoesNotBlockOthers(t *testing.T) {
	withFakeNewMCPClient(t, func(_ context.Context, cfg mcp.Config) (MCPClient, error) {
		if cfg.Command == "bad" {
			return nil, errors.New("connection refused")
		}
		return &fakeMCPClient{tools: []mcp.ToolSchema{{Name: "get_user"}}}, nil
	})

	r := NewRegistry()
	r.RegisterAll(context.Background(), []MCPRegistration{
		{Name: "first-good", Transport: "stdio", Connect: ConnectConfig{Command: "good"}},
		{Name: "broken", Transport: "stdio", Connect: ConnectConfig{Command: "bad"}},
		{Name: "second-good", Transport: "stdio", Connect: ConnectConfig{Command: "good"}},
	})

	good1, ok := r.Get("first-good")
	require.True(t, ok)
	assert.Equal(t, StatusActive, good1.Status)

	broken, ok := r.Get("broken")
	require.True(t, ok)
	assert.Equal(t, StatusUnreachable, broken.Status)

	good2, ok := r.Get("second-good")
	require.True(t, ok)
	assert.Equal(t, StatusActive, good2.Status, "a failure earlier in the list must not short-circuit later registrations")
}

func TestRegistry_RegisterAll_FailureOrderDoesNotMatter(t *testing.T) {
	withFakeNewMCPClient(t, func(_ context.Context, cfg mcp.Config) (MCPClient, error) {
		if cfg.Command == "bad" {
			return nil, errors.New("connection refused")
		}
		return &fakeMCPClient{tools: []mcp.ToolSchema{{Name: "get_user"}}}, nil
	})

	r := NewRegistry()
	// Failure first this time, to confirm order doesn't matter.
	r.RegisterAll(context.Background(), []MCPRegistration{
		{Name: "broken", Transport: "stdio", Connect: ConnectConfig{Command: "bad"}},
		{Name: "good", Transport: "stdio", Connect: ConnectConfig{Command: "good"}},
	})

	broken, _ := r.Get("broken")
	good, _ := r.Get("good")
	assert.Equal(t, StatusUnreachable, broken.Status)
	assert.Equal(t, StatusActive, good.Status)
}

func TestRegistry_RegisterAll_EmptyListIsNoOp(t *testing.T) {
	r := NewRegistry()
	r.RegisterAll(context.Background(), nil)
	assert.Empty(t, r.List())
}
