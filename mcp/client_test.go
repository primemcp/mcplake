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

	var echo *mcp.ToolSchema
	for i := range tools {
		if tools[i].Name == "echo" {
			echo = &tools[i]
		}
	}
	require.NotNil(t, echo, "fixture server should list an echo tool")
	assert.NotEmpty(t, echo.InputSchema)
	assert.True(t, json.Valid(echo.InputSchema))
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

// getenv calls the fixture server's "getenv" tool and decodes its result --
// the one way to observe what environment the real spawned subprocess
// actually has, as opposed to what Config asked for.
func getenv(t *testing.T, client *mcp.Client, ctx context.Context, name string) getenvResult {
	t.Helper()
	resp, err := client.CallTool(ctx, "getenv", map[string]any{"name": name})
	require.NoError(t, err)
	require.False(t, resp.IsError, "getenv tool call reported an error")

	var wrapper struct {
		StructuredContent getenvResult `json:"structuredContent"`
	}
	require.NoError(t, json.Unmarshal(resp.Raw, &wrapper))
	return wrapper.StructuredContent
}

// The security property #166 exists for: a downstream MCP is trusted to
// serve tools, not with the gateway's own secrets. Before this, Config
// left exec.Cmd.Env nil, which os/exec documents as "the new process uses
// the current process's environment" -- so every MCP subprocess could
// read anything the gateway process could, including a persistence DSN
// with a password in it.
func TestNewClient_SubprocessDoesNotInheritTheGatewaysFullEnvironment(t *testing.T) {
	t.Setenv("MCPLAKE_TEST_SECRET", "should-not-leak-to-a-subprocess")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mcp.NewClient(ctx, fixtureConfig())
	require.NoError(t, err)
	defer client.Close()

	got := getenv(t, client, ctx, "MCPLAKE_TEST_SECRET")
	assert.False(t, got.Set, "a variable set only in the gateway's own environment must not reach the subprocess")
}

// The base set is documented and minimal: enough for a child to run and
// resolve its own tooling (PATH to find interpreters/binaries it shells
// out to, HOME for tools that cache under it -- the demo stack's
// bunx-launched MCPs need both), nothing else.
func TestNewClient_SubprocessReceivesTheDocumentedBaseEnvironment(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mcp.NewClient(ctx, fixtureConfig())
	require.NoError(t, err)
	defer client.Close()

	path := getenv(t, client, ctx, "PATH")
	require.True(t, path.Set, "PATH must reach the subprocess -- it's how a child resolves its own interpreters/binaries")
	assert.Equal(t, os.Getenv("PATH"), path.Value, "must be the gateway's real PATH, not an invented one")
}

// An MCP that legitimately needs a credential (an API key, a database DSN
// scoped to itself, not the gateway's own) gets it explicitly, opted in
// per-MCP -- never by inheriting whatever the gateway happens to have.
func TestNewClient_SubprocessReceivesOnlyItsDeclaredEnv(t *testing.T) {
	t.Setenv("MCPLAKE_TEST_SECRET", "should-not-leak-to-a-subprocess")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg := fixtureConfig()
	cfg.Env = map[string]string{"MY_API_KEY": "sk-fixture-only"}
	client, err := mcp.NewClient(ctx, cfg)
	require.NoError(t, err)
	defer client.Close()

	declared := getenv(t, client, ctx, "MY_API_KEY")
	assert.True(t, declared.Set)
	assert.Equal(t, "sk-fixture-only", declared.Value)

	leaked := getenv(t, client, ctx, "MCPLAKE_TEST_SECRET")
	assert.False(t, leaked.Set, "declaring one variable must not open the door to the rest of the gateway's environment")
}

// A declared Env entry is authoritative even when its key collides with
// something the base set would otherwise supply -- the operator named it
// on purpose.
func TestNewClient_DeclaredEnvOverridesTheBaseEnvironment(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg := fixtureConfig()
	cfg.Env = map[string]string{"PATH": "/custom/only/path"}
	client, err := mcp.NewClient(ctx, cfg)
	require.NoError(t, err)
	defer client.Close()

	got := getenv(t, client, ctx, "PATH")
	require.True(t, got.Set)
	assert.Equal(t, "/custom/only/path", got.Value)
}

func TestListTools_CarriesTheToolDescription(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mcp.NewClient(ctx, fixtureConfig())
	require.NoError(t, err)
	defer client.Close()

	tools, err := client.ListTools(ctx)
	require.NoError(t, err)

	var echo *mcp.ToolSchema
	for i := range tools {
		if tools[i].Name == "echo" {
			echo = &tools[i]
		}
	}
	require.NotNil(t, echo, "fixture server should list an echo tool")
	// An MCP client picks a tool by its description, so discovery has to
	// carry it rather than dropping it on the floor. See ADR-0021.
	assert.Equal(t, "echoes the given message back", echo.Description)
}
