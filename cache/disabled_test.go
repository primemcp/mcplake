package cache

import (
	"context"
	"testing"

	"github.com/atsokha/mcplake/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry_Disabled(t *testing.T) {
	r := NewRegistry()
	r.set(MCPRegistration{Name: "on", Status: StatusActive, Enabled: true})
	r.set(MCPRegistration{Name: "off", Status: StatusActive, Enabled: false})

	assert.False(t, r.Disabled("on"), "an enabled registration is not disabled")
	assert.True(t, r.Disabled("off"), "a registered-but-disabled MCP reports disabled")
	assert.False(t, r.Disabled("unknown"), "an unknown MCP is absent, not disabled")
}

func TestRegistry_SetEnabled(t *testing.T) {
	fake := &fakeMCPClient{}
	withFakeNewMCPClient(t, func(context.Context, mcp.Config) (MCPClient, error) { return fake, nil })

	r := NewRegistry()
	require.NoError(t, r.Register(context.Background(), MCPRegistration{
		Name: "postgres-ro", Transport: "stdio", Connect: ConnectConfig{Command: "x"},
		Enabled: true,
	}))

	updated, ok := r.SetEnabled("postgres-ro", false)
	require.True(t, ok)
	assert.False(t, updated.Enabled)
	assert.Equal(t, StatusActive, updated.Status, "SetEnabled does not touch Status")

	assert.True(t, r.Disabled("postgres-ro"))
	client, resolved := r.Resolve("postgres-ro")
	assert.True(t, resolved, "the live client survives a disable")
	assert.Same(t, fake, client)

	_, back := r.SetEnabled("postgres-ro", true)
	assert.True(t, back)
	assert.False(t, r.Disabled("postgres-ro"))

	_, ok = r.SetEnabled("unknown", false)
	assert.False(t, ok)
}

func TestRegistry_Register_CarriesEnabledThrough(t *testing.T) {
	fake := &fakeMCPClient{}
	withFakeNewMCPClient(t, func(context.Context, mcp.Config) (MCPClient, error) { return fake, nil })

	r := NewRegistry()
	err := r.Register(context.Background(), MCPRegistration{
		Name: "seeded-disabled", Transport: "stdio", Connect: ConnectConfig{Command: "x"},
		Enabled: false,
	})
	require.NoError(t, err)

	reg, ok := r.Get("seeded-disabled")
	assert.True(t, ok)
	assert.Equal(t, StatusActive, reg.Status, "it still connects and activates")
	assert.False(t, reg.Enabled, "a registration seeded as disabled stays disabled after Register")
	assert.True(t, r.Disabled("seeded-disabled"))
}
