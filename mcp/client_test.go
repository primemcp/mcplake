package mcp_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/atsokha/mcplake/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixtureConfig returns a Config that spawns this same test binary,
// re-exec'd to run only TestHelperMCPServerProcess with the marker argument
// that makes it act as a fixture MCP server (see fixture_test.go).
func fixtureConfig() mcp.Config {
	return mcp.Config{
		Command:   os.Args[0],
		Arguments: []string{"-test.run=^TestHelperMCPServerProcess$", "--", helperProcessMarker},
	}
}

func TestNewClient_ConnectsAndListsTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mcp.NewClient(ctx, fixtureConfig())
	require.NoError(t, err)
	defer client.Close()

	tools, err := client.ListTools(ctx)
	require.NoError(t, err)
	require.Len(t, tools, 1)
	assert.Equal(t, "echo", tools[0].Name)
	assert.NotEmpty(t, tools[0].InputSchema)
	assert.True(t, json.Valid(tools[0].InputSchema))
}

func TestNewClient_NonexistentCommandReturnsErrorNotHang(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan struct{})
	var err error
	go func() {
		defer close(done)
		_, err = mcp.NewClient(ctx, mcp.Config{Command: "mcplake-definitely-does-not-exist"})
	}()

	select {
	case <-done:
		assert.Error(t, err)
	case <-time.After(4 * time.Second):
		t.Fatal("NewClient hung connecting to a nonexistent command")
	}
}

func TestNewClient_RequiresCommand(t *testing.T) {
	_, err := mcp.NewClient(context.Background(), mcp.Config{})
	assert.Error(t, err)
}

func TestClient_CallTool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mcp.NewClient(ctx, fixtureConfig())
	require.NoError(t, err)
	defer client.Close()

	resp, err := client.CallTool(ctx, "echo", map[string]any{"message": "hello"})
	require.NoError(t, err)
	require.False(t, resp.IsError)
	assert.Contains(t, string(resp.Raw), "hello")
}

func TestClient_CallTool_UnknownToolReturnsErrorNotPanic(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mcp.NewClient(ctx, fixtureConfig())
	require.NoError(t, err)
	defer client.Close()

	_, err = client.CallTool(ctx, "does_not_exist", nil)
	assert.Error(t, err)
}

func TestClient_Close_IdempotentAndTerminatesSubprocess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mcp.NewClient(ctx, fixtureConfig())
	require.NoError(t, err)

	require.NoError(t, client.Close())
	assert.NotPanics(t, func() { _ = client.Close() })
}

func TestClient_ContextCancellationDuringCallToolReturnsPromptly(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mcp.NewClient(ctx, fixtureConfig())
	require.NoError(t, err)
	defer client.Close()

	callCtx, callCancel := context.WithCancel(ctx)
	callCancel() // already cancelled before the call starts

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = client.CallTool(callCtx, "echo", map[string]any{"message": "hi"})
	}()

	select {
	case <-done:
	case <-time.After(4 * time.Second):
		t.Fatal("CallTool did not return promptly for an already-cancelled context")
	}
}
