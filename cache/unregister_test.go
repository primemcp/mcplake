package cache

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/atsokha/mcplake/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry_Unregister_RemovesEntryAndClosesClient(t *testing.T) {
	fake := &fakeMCPClient{tools: []mcp.ToolSchema{{Name: "get_user"}}}
	withFakeNewMCPClient(t, func(context.Context, mcp.Config) (MCPClient, error) { return fake, nil })

	r := NewRegistry()
	require.NoError(t, r.Register(context.Background(), MCPRegistration{
		Name: "postgres-ro", Transport: "stdio", Connect: ConnectConfig{Command: "x"},
	}))

	require.NoError(t, r.Unregister("postgres-ro"))

	_, ok := r.Get("postgres-ro")
	assert.False(t, ok)
	assert.False(t, r.HasTool("postgres-ro", "get_user"))
	assert.Empty(t, r.List())
	assert.True(t, fake.closed)
}

func TestRegistry_Unregister_UnknownNameReturnsError(t *testing.T) {
	r := NewRegistry()
	err := r.Unregister("does-not-exist")
	assert.Error(t, err)
}

func TestRegistry_Register_Reregistration_ReplacesToolsAndClosesOldClient(t *testing.T) {
	oldClient := &fakeMCPClient{tools: []mcp.ToolSchema{{Name: "old_tool"}}}
	newClient := &fakeMCPClient{tools: []mcp.ToolSchema{{Name: "new_tool"}}}
	calls := 0
	withFakeNewMCPClient(t, func(context.Context, mcp.Config) (MCPClient, error) {
		calls++
		if calls == 1 {
			return oldClient, nil
		}
		return newClient, nil
	})

	r := NewRegistry()
	reg := MCPRegistration{Name: "postgres-ro", Transport: "stdio", Connect: ConnectConfig{Command: "x"}}
	require.NoError(t, r.Register(context.Background(), reg))
	require.NoError(t, r.Register(context.Background(), reg)) // re-registration

	got, ok := r.Get("postgres-ro")
	require.True(t, ok)
	assert.Same(t, newClient, got.Client)
	assert.True(t, r.HasTool("postgres-ro", "new_tool"))
	assert.False(t, r.HasTool("postgres-ro", "old_tool"), "re-registration must replace the tool set wholesale, not merge it")
	assert.True(t, oldClient.closed, "the superseded client must be closed after a successful re-registration")
	assert.False(t, newClient.closed)
}

func TestRegistry_Register_FailedReregistrationLeavesOldRegistrationIntact(t *testing.T) {
	oldClient := &fakeMCPClient{tools: []mcp.ToolSchema{{Name: "get_user"}}}
	calls := 0
	withFakeNewMCPClient(t, func(context.Context, mcp.Config) (MCPClient, error) {
		calls++
		if calls == 1 {
			return oldClient, nil
		}
		return nil, errors.New("new connection attempt failed")
	})

	r := NewRegistry()
	reg := MCPRegistration{Name: "postgres-ro", Transport: "stdio", Connect: ConnectConfig{Command: "x"}}
	require.NoError(t, r.Register(context.Background(), reg))

	err := r.Register(context.Background(), reg) // re-registration attempt fails
	require.Error(t, err)

	got, ok := r.Get("postgres-ro")
	require.True(t, ok)
	assert.Equal(t, StatusActive, got.Status, "a failed re-registration must not clobber the existing working registration")
	assert.Same(t, oldClient, got.Client)
	assert.True(t, r.HasTool("postgres-ro", "get_user"))
	assert.False(t, oldClient.closed, "the still-working old client must not be closed by a failed re-registration attempt")
}

func TestRegistry_Register_FailedReregistrationOnToolDiscoveryLeavesOldRegistrationIntact(t *testing.T) {
	oldClient := &fakeMCPClient{tools: []mcp.ToolSchema{{Name: "get_user"}}}
	newClient := &fakeMCPClient{listErr: errors.New("tools/list failed")}
	calls := 0
	withFakeNewMCPClient(t, func(context.Context, mcp.Config) (MCPClient, error) {
		calls++
		if calls == 1 {
			return oldClient, nil
		}
		return newClient, nil
	})

	r := NewRegistry()
	reg := MCPRegistration{Name: "postgres-ro", Transport: "stdio", Connect: ConnectConfig{Command: "x"}}
	require.NoError(t, r.Register(context.Background(), reg))

	err := r.Register(context.Background(), reg)
	require.Error(t, err)

	got, ok := r.Get("postgres-ro")
	require.True(t, ok)
	assert.Equal(t, StatusActive, got.Status)
	assert.Same(t, oldClient, got.Client)
	assert.False(t, oldClient.closed)
	assert.True(t, newClient.closed, "the failed new client must still be closed itself, just not the old one")
}

func TestRegistry_ConcurrentReadsRaceFreeDuringReregistration(t *testing.T) {
	callN := 0
	var mu sync.Mutex
	withFakeNewMCPClient(t, func(context.Context, mcp.Config) (MCPClient, error) {
		mu.Lock()
		callN++
		n := callN
		mu.Unlock()
		return &fakeMCPClient{tools: []mcp.ToolSchema{{Name: fmt.Sprintf("tool-%d", n)}}}, nil
	})

	r := NewRegistry()
	reg := MCPRegistration{Name: "postgres-ro", Transport: "stdio", Connect: ConnectConfig{Command: "x"}}
	require.NoError(t, r.Register(context.Background(), reg))

	var wg sync.WaitGroup
	for range 50 {
		wg.Add(3)
		go func() {
			defer wg.Done()
			_ = r.Register(context.Background(), reg)
		}()
		go func() {
			defer wg.Done()
			r.Get("postgres-ro")
		}()
		go func() {
			defer wg.Done()
			r.List()
		}()
	}
	wg.Wait()
}
