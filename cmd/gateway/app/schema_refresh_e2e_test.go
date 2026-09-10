package app_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/atsokha/mcplake/config"
	"github.com/golang-jwt/jwt/v5"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// growingMCPMarker is the sentinel trailing arg that turns
// TestHelperGrowingMCPProcess into a fixture MCP server.
const growingMCPMarker = "MCPLAKE_APP_GROWING_MCP"

// growingSentinelEnv names the file whose presence toggles the fixture's
// second tool ("beta") on and off.
const growingSentinelEnv = "MCPLAKE_APP_GROWING_SENTINEL"

type growingToolOutput struct {
	OK bool `json:"ok"`
}

// TestHelperGrowingMCPProcess is not a real test: run normally it skips.
// Re-exec'd with growingMCPMarker it is a stdio MCP server that always
// advertises "alpha" and additionally advertises "beta" whenever the file
// named by $MCPLAKE_APP_GROWING_SENTINEL exists — a downstream whose tool set
// changes while the gateway is running.
func TestHelperGrowingMCPProcess(t *testing.T) {
	if len(os.Args) == 0 || os.Args[len(os.Args)-1] != growingMCPMarker {
		t.Skip("not invoked as the growing MCP fixture helper process")
	}

	sentinel := os.Getenv(growingSentinelEnv)
	server := sdk.NewServer(&sdk.Implementation{Name: "mcplake-e2e-growing", Version: "0.1.0"}, nil)

	ok := func(_ context.Context, _ *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, growingToolOutput, error) {
		return nil, growingToolOutput{OK: true}, nil
	}
	sdk.AddTool(server, &sdk.Tool{Name: "alpha", Description: "always present"}, ok)

	go func() {
		present := false
		for {
			_, err := os.Stat(sentinel)
			want := err == nil
			switch {
			case want && !present:
				sdk.AddTool(server, &sdk.Tool{Name: "beta", Description: "present only while the sentinel exists"}, ok)
			case !want && present:
				server.RemoveTools("beta")
			}
			present = want
			time.Sleep(20 * time.Millisecond)
		}
	}()

	if err := server.Run(context.Background(), &sdk.StdioTransport{}); err != nil {
		fmt.Fprintln(os.Stderr, "growing fixture server exited:", err)
		os.Exit(1)
	}
	os.Exit(0)
}

func TestApp_SchemaRefresh_EndToEnd(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	jwks := newJWKSTestServer(t, &key.PublicKey)
	defer jwks.Close()

	sentinel := filepath.Join(t.TempDir(), "beta.on")
	t.Setenv(growingSentinelEnv, sentinel) // inherited by the fixture subprocess

	cfg := testConfig(t, jwks.URL)
	cfg.MCP.SchemaRefreshInterval = config.Duration{Duration: 100 * time.Millisecond}
	cfg.MCPs = []config.MCPConfig{{
		Name:      "growing",
		Type:      "stdio",
		Command:   os.Args[0],
		Arguments: []string{"-test.run=^TestHelperGrowingMCPProcess$", "--", growingMCPMarker},
	}}
	cfg.AccessPolicies = []config.AccessPolicyConfig{{
		Name:   "growing-users",
		Match:  []config.ClaimRuleConfig{{Path: "$.role", Pattern: "^grower$"}},
		Grants: []config.GrantConfig{{MCP: "growing", Tools: []string{"*"}}},
	}}

	dataAddr, _, cleanup := startTestApp(t, cfg)
	defer cleanup()

	now := time.Now()
	token := signToken(t, key, jwt.MapClaims{
		"iss": testIssuer, "aud": testAudience, "sub": "u", "role": "grower",
		"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
	})

	call := func(tool string) (int, string) {
		body, _ := json.Marshal(map[string]any{"mcp": "growing", "tool": tool, "arguments": map[string]any{}})
		req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s/v1/call", dataAddr), bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		return resp.StatusCode, e.Error
	}

	// waitFor polls call(tool) until pred(status, code) or a deadline.
	waitFor := func(tool string, pred func(int, string) bool, why string) {
		t.Helper()
		deadline := time.Now().Add(4 * time.Second)
		for time.Now().Before(deadline) {
			if status, code := call(tool); pred(status, code) {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		status, code := call(tool)
		t.Fatalf("timed out waiting for %s (last: %d %q)", why, status, code)
	}

	// alpha is callable from the start.
	status, code := call("alpha")
	require.Equal(t, http.StatusOK, status, "alpha should be callable at boot (code %q)", code)

	// beta is not advertised yet.
	status, code = call("beta")
	require.Equal(t, http.StatusNotFound, status)
	require.Equal(t, "tool_not_found", code)

	// Downstream gains beta; the gateway picks it up within ~one interval,
	// no operator action, no reconnect.
	require.NoError(t, os.WriteFile(sentinel, []byte("x"), 0o600))
	waitFor("beta", func(s int, _ string) bool { return s == http.StatusOK }, "beta to become callable after the tool set grew")

	// alpha still works through the refresh.
	status, code = call("alpha")
	assert.Equal(t, http.StatusOK, status, "alpha still callable after a refresh (code %q)", code)

	// Downstream drops beta again; the gateway stops routing it.
	require.NoError(t, os.Remove(sentinel))
	waitFor("beta", func(s int, c string) bool { return s == http.StatusNotFound && c == "tool_not_found" }, "beta to stop being callable after the tool set shrank")
}
