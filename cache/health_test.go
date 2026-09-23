package cache

import (
	"context"
	"errors"
	"testing"

	"github.com/atsokha/mcplake/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unreachableReg stores a registration in the state Register leaves behind
// when it could never connect, and demote leaves behind when a live session
// dies: no client, StatusUnreachable, identity and operator intent intact.
func unreachableReg(r *Registry, name string, connect ConnectConfig, enabled bool) {
	r.set(MCPRegistration{
		Name:      name,
		Transport: TransportHTTP,
		Connect:   connect,
		Status:    StatusUnreachable,
		Enabled:   enabled,
	})
}

func TestCheckHealth_UnknownMCPReportsNotRegistered(t *testing.T) {
	r := NewRegistry()

	_, ok := r.CheckHealth(t.Context(), "nope")

	assert.False(t, ok)
}

func TestCheckHealth_LiveSessionIsLeftAlone(t *testing.T) {
	r := NewRegistry()
	client := &fakeMCPClient{tools: []mcp.ToolSchema{{Name: "get_user"}}}
	activeReg(r, "pg", client, "get_user")
	withFakeNewMCPClient(t, func(context.Context, mcp.Config) (MCPClient, error) {
		t.Fatal("a healthy session must not be reconnected")
		return nil, nil
	})

	res, ok := r.CheckHealth(t.Context(), "pg")

	require.True(t, ok)
	assert.Equal(t, StatusActive, res.Status)
	assert.False(t, res.Reconnected)
	assert.NoError(t, res.Err)
	assert.Equal(t, 1, client.pings, "a healthy check is exactly one ping")

	got, _ := r.Get("pg")
	assert.Same(t, client, got.Client, "the working client must be kept")
	assert.False(t, client.closed)
}

// A single failed ping is usually a pooled TCP connection the server closed,
// not a dead session — the gateway's http transport holds an idle connection
// far longer than a typical server does. Rebuilding the session for that
// would throw away a working one (and re-run tools/list) on every idle gap.
func TestCheckHealth_TransientPingFailureIsRetriedNotReconnected(t *testing.T) {
	r := NewRegistry()
	client := &fakeMCPClient{pingFailures: 1, tools: []mcp.ToolSchema{{Name: "get_user"}}}
	activeReg(r, "pg", client, "get_user")
	withFakeNewMCPClient(t, func(context.Context, mcp.Config) (MCPClient, error) {
		t.Fatal("a session that answers the retry must not be reconnected")
		return nil, nil
	})

	res, ok := r.CheckHealth(t.Context(), "pg")

	require.True(t, ok)
	assert.Equal(t, StatusActive, res.Status)
	assert.False(t, res.Reconnected)
	assert.Equal(t, 2, client.pings, "one failure, then the retry that succeeded")

	got, _ := r.Get("pg")
	assert.Same(t, client, got.Client)
	assert.False(t, client.closed)
}

// ...but a session that fails the retry too is genuinely gone.
func TestCheckHealth_PingFailingTwiceTriggersAReconnect(t *testing.T) {
	r := NewRegistry()
	dead := &fakeMCPClient{pingErr: errors.New("connection refused")}
	activeReg(r, "pg", dead, "get_user")
	fresh := &fakeMCPClient{tools: []mcp.ToolSchema{{Name: "get_user"}}}
	withFakeNewMCPClient(t, func(context.Context, mcp.Config) (MCPClient, error) {
		return fresh, nil
	})

	res, ok := r.CheckHealth(t.Context(), "pg")

	require.True(t, ok)
	assert.True(t, res.Reconnected)
	assert.Equal(t, healthProbeAttempts, dead.pings)

	got, _ := r.Get("pg")
	assert.Same(t, fresh, got.Client)
}

// The ADR-0019 headline: a downstream that restarted underneath a live
// registration is repaired without the registration ever leaving
// StatusActive, so a concurrent caller sees the old client or the new one,
// never a gap.
func TestCheckHealth_DeadSessionIsReconnectedInPlace(t *testing.T) {
	r := NewRegistry()
	dead := &fakeMCPClient{pingErr: errors.New("connection reset by peer")}
	activeReg(r, "pg", dead, "get_user")

	fresh := &fakeMCPClient{tools: []mcp.ToolSchema{{Name: "get_user"}, {Name: "list_users"}}}
	withFakeNewMCPClient(t, func(context.Context, mcp.Config) (MCPClient, error) {
		return fresh, nil
	})

	res, ok := r.CheckHealth(t.Context(), "pg")

	require.True(t, ok)
	assert.Equal(t, StatusActive, res.Status)
	assert.True(t, res.Reconnected)
	assert.NoError(t, res.Err)

	got, _ := r.Get("pg")
	assert.Same(t, fresh, got.Client)
	assert.True(t, dead.closed, "the superseded client must be closed")
	assert.True(t, r.HasTool("pg", "list_users"), "tools are re-discovered on reconnect")
	client, resolvable := r.Resolve("pg")
	assert.True(t, resolvable)
	assert.Same(t, fresh, client)
}

// The other half of #191: a registration that failed at startup becomes
// usable on its own once its server arrives, with no operator action.
func TestCheckHealth_UnreachableRegistrationIsRevived(t *testing.T) {
	r := NewRegistry()
	unreachableReg(r, "remote", ConnectConfig{URL: "http://127.0.0.1:1/mcp"}, true)

	var gotCfg mcp.Config
	fresh := &fakeMCPClient{tools: []mcp.ToolSchema{{Name: "get_user"}}}
	withFakeNewMCPClient(t, func(_ context.Context, cfg mcp.Config) (MCPClient, error) {
		gotCfg = cfg
		return fresh, nil
	})

	res, ok := r.CheckHealth(t.Context(), "remote")

	require.True(t, ok)
	assert.Equal(t, StatusActive, res.Status)
	assert.True(t, res.Reconnected)

	got, _ := r.Get("remote")
	assert.Equal(t, StatusActive, got.Status)
	assert.Same(t, fresh, got.Client)
	assert.True(t, r.HasTool("remote", "get_user"))

	// The reconnect must dial what the registration says, not a default.
	assert.Equal(t, TransportHTTP, gotCfg.Transport)
	assert.Equal(t, "http://127.0.0.1:1/mcp", gotCfg.URL)
}

func TestCheckHealth_ReconnectFailureDemotesButKeepsSchemas(t *testing.T) {
	r := NewRegistry()
	dead := &fakeMCPClient{pingErr: errors.New("broken pipe")}
	activeReg(r, "pg", dead, "get_user")
	withFakeNewMCPClient(t, func(context.Context, mcp.Config) (MCPClient, error) {
		return nil, errors.New("connection refused")
	})

	res, ok := r.CheckHealth(t.Context(), "pg")

	require.True(t, ok)
	assert.Equal(t, StatusUnreachable, res.Status)
	assert.False(t, res.Reconnected)
	require.Error(t, res.Err)
	assert.Contains(t, res.Err.Error(), "connection refused")

	got, _ := r.Get("pg")
	assert.Equal(t, StatusUnreachable, got.Status)
	assert.Nil(t, got.Client, "a client that cannot carry a call must not be resolvable")
	assert.True(t, dead.closed)

	// Kept, so the admin UI can still show and edit filters for an endpoint
	// that is merely down — Status is what makes it uncallable.
	assert.Contains(t, got.Tools, "get_user")
	assert.False(t, r.HasTool("pg", "get_user"), "a non-active MCP advertises nothing callable")
	_, resolvable := r.Resolve("pg")
	assert.False(t, resolvable)
	assert.True(t, r.Registered("pg"), "it is down, not gone")
}

// A tools/list that fails after the connect succeeded is the same outcome
// as a failed connect: the half-built client is closed rather than stored.
func TestCheckHealth_RediscoveryFailureClosesTheNewClient(t *testing.T) {
	r := NewRegistry()
	unreachableReg(r, "remote", ConnectConfig{URL: "https://mcp.example.com/mcp"}, true)

	fresh := &fakeMCPClient{listErr: errors.New("tools/list exploded")}
	withFakeNewMCPClient(t, func(context.Context, mcp.Config) (MCPClient, error) {
		return fresh, nil
	})

	res, ok := r.CheckHealth(t.Context(), "remote")

	require.True(t, ok)
	assert.Equal(t, StatusUnreachable, res.Status)
	require.Error(t, res.Err)
	assert.True(t, fresh.closed, "a client whose discovery failed must not leak")

	got, _ := r.Get("remote")
	assert.Nil(t, got.Client)
}

// Enabled is operator intent, not connection state: a disabled MCP that is
// healed must stay disabled, or a reconnect silently re-opens an endpoint
// somebody turned off.
func TestCheckHealth_ReconnectPreservesDisabled(t *testing.T) {
	r := NewRegistry()
	unreachableReg(r, "remote", ConnectConfig{URL: "https://mcp.example.com/mcp"}, false)
	withFakeNewMCPClient(t, func(context.Context, mcp.Config) (MCPClient, error) {
		return &fakeMCPClient{tools: []mcp.ToolSchema{{Name: "get_user"}}}, nil
	})

	_, ok := r.CheckHealth(t.Context(), "remote")
	require.True(t, ok)

	got, _ := r.Get("remote")
	assert.Equal(t, StatusActive, got.Status)
	assert.False(t, got.Enabled)
	assert.True(t, r.Disabled("remote"))
}

// A reconnect that started before a concurrent Register must not overwrite
// it — the admin's newer registration is the one the operator meant.
func TestCheckHealth_ConcurrentRegisterWinsOverAnInFlightReconnect(t *testing.T) {
	r := NewRegistry()
	dead := &fakeMCPClient{pingErr: errors.New("gone")}
	activeReg(r, "pg", dead, "get_user")

	winner := &fakeMCPClient{tools: []mcp.ToolSchema{{Name: "winner"}}}
	loser := &fakeMCPClient{tools: []mcp.ToolSchema{{Name: "loser"}}}
	withFakeNewMCPClient(t, func(context.Context, mcp.Config) (MCPClient, error) {
		return loser, nil
	})

	// Swap a different client in while the reconnect is between its
	// ListTools and its store, which is exactly what the fake's hook
	// simulates.
	loser.onListTools = func() { activeReg(r, "pg", winner, "winner") }

	res, ok := r.CheckHealth(t.Context(), "pg")

	require.True(t, ok)
	assert.NoError(t, res.Err, "losing the race is not an error, just a discarded result")

	got, _ := r.Get("pg")
	assert.Same(t, winner, got.Client)
	assert.True(t, loser.closed, "the discarded client must not leak")
	assert.True(t, r.HasTool("pg", "winner"))
	assert.False(t, r.HasTool("pg", "loser"))
}

func TestCheckHealth_UnregisteredMidReconnectIsDiscarded(t *testing.T) {
	r := NewRegistry()
	unreachableReg(r, "remote", ConnectConfig{URL: "https://mcp.example.com/mcp"}, true)

	fresh := &fakeMCPClient{tools: []mcp.ToolSchema{{Name: "get_user"}}}
	fresh.onListTools = func() { _ = r.Unregister("remote") }
	withFakeNewMCPClient(t, func(context.Context, mcp.Config) (MCPClient, error) {
		return fresh, nil
	})

	_, ok := r.CheckHealth(t.Context(), "remote")
	require.True(t, ok)

	assert.False(t, r.Registered("remote"), "Unregister wins")
	assert.True(t, fresh.closed, "the orphaned client must be closed")
}

func TestCheckAll_VisitsEveryRegistrationAndIsolatesFailures(t *testing.T) {
	r := NewRegistry()
	healthy := &fakeMCPClient{tools: []mcp.ToolSchema{{Name: "ok"}}}
	activeReg(r, "healthy", healthy, "ok")
	activeReg(r, "broken", &fakeMCPClient{pingErr: errors.New("gone")}, "ok")
	unreachableReg(r, "down", ConnectConfig{URL: "https://mcp.example.com/mcp"}, true)

	withFakeNewMCPClient(t, func(context.Context, mcp.Config) (MCPClient, error) {
		return nil, errors.New("connection refused")
	})

	results := r.CheckAll(t.Context())

	require.Len(t, results, 3)
	byName := make(map[string]HealthResult, len(results))
	for _, res := range results {
		byName[res.Name] = res
	}
	assert.NoError(t, byName["healthy"].Err)
	assert.Equal(t, StatusActive, byName["healthy"].Status)
	assert.Error(t, byName["broken"].Err)
	assert.Error(t, byName["down"].Err)
}

func TestNames_ReturnsEveryRegistrationSorted(t *testing.T) {
	r := NewRegistry()
	activeReg(r, "zulu", &fakeMCPClient{}, "t")
	activeReg(r, "alpha", &fakeMCPClient{}, "t")
	unreachableReg(r, "mike", ConnectConfig{URL: "https://mcp.example.com/mcp"}, true)

	assert.Equal(t, []string{"alpha", "mike", "zulu"}, r.Names())
}

// A cancelled context means the gateway is shutting down, not that every
// downstream vanished at once. Reconnecting then would demote the lot and
// log a warning each, describing nothing that actually happened.
func TestCheckHealth_ShutdownDoesNotDemoteEverything(t *testing.T) {
	r := NewRegistry()
	client := &fakeMCPClient{pingErr: context.Canceled}
	activeReg(r, "pg", client, "get_user")
	withFakeNewMCPClient(t, func(context.Context, mcp.Config) (MCPClient, error) {
		t.Fatal("no reconnect should be attempted once the context is done")
		return nil, nil
	})

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	res, ok := r.CheckHealth(ctx, "pg")

	require.True(t, ok)
	assert.Equal(t, StatusActive, res.Status)
	assert.NoError(t, res.Err)

	got, _ := r.Get("pg")
	assert.Equal(t, StatusActive, got.Status, "shutdown must not rewrite registration state")
	assert.False(t, client.closed)
}
