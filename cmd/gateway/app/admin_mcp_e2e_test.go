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
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/primemcp/mcplake/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bearerRoundTripper attaches a fixed Bearer token to every request, so the
// go-sdk streamable client can authenticate against the admin-auth gate.
type bearerRoundTripper struct {
	base  http.RoundTripper
	token string
}

func (b bearerRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	if b.token != "" {
		clone.Header.Set("Authorization", "Bearer "+b.token)
	}
	return b.base.RoundTrip(clone)
}

func adminMCPConfig(t *testing.T, jwksURL string) *config.Config {
	t.Helper()
	cfg := adminAuthConfig(t, jwksURL) // admin_auth.match: role == "admin"
	cfg.AdminMCP = config.AdminMCPConfig{Enabled: boolPtr(true)}
	return cfg
}

func connectAdminMCP(t *testing.T, controlAddr, token string) *sdk.ClientSession {
	t.Helper()
	client := sdk.NewClient(&sdk.Implementation{Name: "e2e-ops", Version: "0"}, nil)
	tr := &sdk.StreamableClientTransport{
		Endpoint:             fmt.Sprintf("http://%s/admin/mcp", controlAddr),
		HTTPClient:           &http.Client{Transport: bearerRoundTripper{http.DefaultTransport, token}},
		DisableStandaloneSSE: true,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	session, err := client.Connect(ctx, tr, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func mcpCall(t *testing.T, cs *sdk.ClientSession, name string, args any) *sdk.CallToolResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	res, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	return res
}

func mcpDecode(t *testing.T, res *sdk.CallToolResult, target any) {
	t.Helper()
	raw, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, target))
}

func TestApp_AdminMCP_EndToEnd(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	jwks := newJWKSTestServer(t, &key.PublicKey)
	defer jwks.Close()

	dataAddr, controlAddr, cleanup := startTestApp(t, adminMCPConfig(t, jwks.URL))
	t.Cleanup(cleanup)

	now := time.Now()
	mkToken := func(role string) string {
		return signToken(t, key, jwt.MapClaims{
			"iss": testIssuer, "aud": testAudience, "sub": role, "role": role,
			"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
		})
	}
	adminToken := mkToken("admin")

	// A POST /v1/call helper returning status + decoded error code (if any).
	dataPlaneCall := func(token, mcp, tool string) (int, string) {
		body, _ := json.Marshal(map[string]any{"mcp": mcp, "tool": tool, "arguments": map[string]any{"message": "hi"}})
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

	restGET := func(path string) string {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s%s", controlAddr, path), nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		raw, _ := io.ReadAll(resp.Body)
		return string(raw)
	}

	t.Run("connection without an admin token is rejected", func(t *testing.T) {
		client := sdk.NewClient(&sdk.Implementation{Name: "e2e-noauth", Version: "0"}, nil)
		tr := &sdk.StreamableClientTransport{Endpoint: fmt.Sprintf("http://%s/admin/mcp", controlAddr), DisableStandaloneSSE: true}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err := client.Connect(ctx, tr, nil)
		require.Error(t, err)
	})

	cs := connectAdminMCP(t, controlAddr, adminToken)

	// register_mcp -> the echo fixture, then grant a caller role over MCP.
	echoArgs := []string{"-test.run=^TestHelperEchoMCPProcess$", "--", echoMCPMarker}
	reg := mcpCall(t, cs, "register_mcp", map[string]any{
		"name": "echo-mcp", "command": os.Args[0], "arguments": echoArgs,
	})
	require.False(t, reg.IsError, "register_mcp: %v", reg.Content)

	grant := mcpCall(t, cs, "create_access_policy", map[string]any{
		"name":   "echo-users",
		"match":  []map[string]any{{"path": "$.role", "pattern": "^echo-user$"}},
		"grants": []map[string]any{{"mcp": "echo-mcp", "tools": []string{"*"}}},
	})
	require.False(t, grant.IsError, "create_access_policy: %v", grant.Content)

	// list_mcps over MCP, and the same registration over REST.
	var listed struct {
		MCPs []struct {
			Name    string `json:"name"`
			Enabled bool   `json:"enabled"`
		} `json:"mcps"`
	}
	mcpDecode(t, mcpCall(t, cs, "list_mcps", map[string]any{}), &listed)
	require.Len(t, listed.MCPs, 1)
	assert.Equal(t, "echo-mcp", listed.MCPs[0].Name)
	assert.Contains(t, restGET("/admin/mcps"), "echo-mcp")

	caller := mkToken("echo-user")

	// Enabled: the data plane routes the call through to the fixture.
	status, _ := dataPlaneCall(caller, "echo-mcp", "echo")
	assert.Equal(t, http.StatusOK, status, "registered+granted MCP is callable via the data plane")

	// set_mcp_enabled false over MCP -> data plane now rejects with mcp_disabled.
	dis := mcpCall(t, cs, "set_mcp_enabled", map[string]any{"name": "echo-mcp", "enabled": false})
	require.False(t, dis.IsError, "%v", dis.Content)
	status, code := dataPlaneCall(caller, "echo-mcp", "echo")
	assert.Equal(t, http.StatusForbidden, status)
	assert.Equal(t, "mcp_disabled", code)

	// Re-enable, then unregister -> data plane no longer knows it.
	en := mcpCall(t, cs, "set_mcp_enabled", map[string]any{"name": "echo-mcp", "enabled": true})
	require.False(t, en.IsError, "%v", en.Content)
	status, _ = dataPlaneCall(caller, "echo-mcp", "echo")
	assert.Equal(t, http.StatusOK, status)

	del := mcpCall(t, cs, "unregister_mcp", map[string]any{"name": "echo-mcp"})
	require.False(t, del.IsError, "%v", del.Content)
	var empty struct {
		MCPs []json.RawMessage `json:"mcps"`
	}
	mcpDecode(t, mcpCall(t, cs, "list_mcps", map[string]any{}), &empty)
	assert.Empty(t, empty.MCPs)

	// Access-policy CRUD round-trip via tools, each step checked over REST.
	require.False(t, mcpCall(t, cs, "create_access_policy", map[string]any{
		"name":   "db-reader",
		"match":  []map[string]any{{"path": "$.role", "pattern": "^db-reader$"}},
		"grants": []map[string]any{{"mcp": "postgres-ro", "tools": []string{"*"}}},
	}).IsError)
	assert.Contains(t, restGET("/admin/access-policies"), "db-reader")

	require.False(t, mcpCall(t, cs, "get_access_policy", map[string]any{"name": "db-reader"}).IsError)
	require.False(t, mcpCall(t, cs, "replace_access_policy", map[string]any{
		"name":   "db-reader",
		"match":  []map[string]any{{"path": "$.role", "pattern": "^db-reader$"}},
		"grants": []map[string]any{{"mcp": "postgres-rw", "tools": []string{"get_user"}}},
	}).IsError)
	require.False(t, mcpCall(t, cs, "delete_access_policy", map[string]any{"name": "db-reader"}).IsError)
	assert.NotContains(t, restGET("/admin/access-policies"), "db-reader")
	assert.True(t, mcpCall(t, cs, "get_access_policy", map[string]any{"name": "db-reader"}).IsError)
}
