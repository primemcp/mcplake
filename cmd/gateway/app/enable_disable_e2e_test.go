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

// echoMCPMarker is the sentinel trailing argument that makes
// TestHelperEchoMCPProcess run as a fixture MCP server rather than skip.
// Same re-exec-the-test-binary pattern as mcp/fixture_test.go.
const echoMCPMarker = "MCPLAKE_APP_ECHO_MCP"

type echoInput struct {
	Message string `json:"message" jsonschema:"the message to echo back"`
}

type echoOutput struct {
	Message string `json:"message"`
	Secret  string `json:"secret"`
}

// TestHelperEchoMCPProcess is not a real test: invoked normally it skips.
// The e2e test below re-execs the test binary pinned to this function plus
// echoMCPMarker, turning it into a minimal stdio MCP server exposing one
// tool, "echo", whose structured result carries a "secret" field for a
// filter policy to (not) strip.
func TestHelperEchoMCPProcess(t *testing.T) {
	if len(os.Args) == 0 || os.Args[len(os.Args)-1] != echoMCPMarker {
		t.Skip("not invoked as the echo MCP fixture helper process")
	}

	server := sdk.NewServer(&sdk.Implementation{Name: "mcplake-e2e-echo", Version: "0.1.0"}, nil)
	sdk.AddTool(server, &sdk.Tool{
		Name:        "echo",
		Description: "echoes the given message back, plus a secret",
	}, func(_ context.Context, _ *sdk.CallToolRequest, in echoInput) (*sdk.CallToolResult, echoOutput, error) {
		return nil, echoOutput{Message: in.Message, Secret: "s3cr3t"}, nil
	})

	if err := server.Run(context.Background(), &sdk.StdioTransport{}); err != nil {
		fmt.Fprintln(os.Stderr, "echo fixture server exited:", err)
		os.Exit(1)
	}
	os.Exit(0)
}

func boolPtr(b bool) *bool { return &b }

// echoedMessage is deliberately distinctive: the filter assertions below
// search the whole response body for it, so it must not collide with any
// other token in the envelope.
const echoedMessage = "e2e-echo-marker-9f2c"

// e2eConfig builds a Config seeded with disabled entries of all three kinds:
//   - mcp-off: an MCP with enabled=false (mcp-live is enabled)
//   - access-ghost: an access policy with enabled=false, the only grant for
//     role "ghost"
//   - filter-off: a filter policy with enabled=false that would strip
//     $.secret from mcp-live/echo
//
// filter-live (enabled) strips $.message, so the enabled path is exercised
// in the same response. Both paths are record-relative -- the form the docs
// show -- not envelope-relative; see ADR-0015.
func e2eConfig(t *testing.T, jwksURL string) *config.Config {
	t.Helper()
	echoCmd := os.Args[0]
	echoArgs := []string{"-test.run=^TestHelperEchoMCPProcess$", "--", echoMCPMarker}

	return &config.Config{
		Server: config.ServerConfig{DataPlaneAddr: "127.0.0.1:0", ControlPlaneAddr: "127.0.0.1:0"},
		OIDC:   config.OIDCConfig{JWKSURL: jwksURL, Issuer: testIssuer, Audience: testAudience},
		Persistence: config.PersistenceConfig{
			DSN: filepath.Join(t.TempDir(), "e2e.db"),
		},
		MCPs: []config.MCPConfig{
			{Name: "mcp-live", Command: echoCmd, Arguments: echoArgs},
			{Name: "mcp-off", Command: echoCmd, Arguments: echoArgs, Enabled: boolPtr(false)},
		},
		AccessPolicies: []config.AccessPolicyConfig{
			{
				Name:  "access-user",
				Match: []config.ClaimRuleConfig{{Path: "$.role", Pattern: "^user$"}},
				Grants: []config.GrantConfig{
					{MCP: "mcp-live", Tools: []string{"*"}},
					{MCP: "mcp-off", Tools: []string{"*"}},
				},
			},
			{
				Name:    "access-ghost",
				Enabled: boolPtr(false),
				Match:   []config.ClaimRuleConfig{{Path: "$.role", Pattern: "^ghost$"}},
				Grants:  []config.GrantConfig{{MCP: "mcp-live", Tools: []string{"*"}}},
			},
		},
		FilterPolicies: []config.FilterPolicyConfig{
			{
				Name:       "filter-off",
				Enabled:    boolPtr(false),
				Match:      []config.ClaimRuleConfig{{Path: "$.role", Pattern: "^user$"}},
				MCP:        "mcp-live",
				Tool:       "echo",
				DropFields: []string{"$.secret"},
			},
			{
				Name:       "filter-live",
				Match:      []config.ClaimRuleConfig{{Path: "$.role", Pattern: "^user$"}},
				MCP:        "mcp-live",
				Tool:       "echo",
				DropFields: []string{"$.message"},
			},
		},
	}
}

// TestApp_EnableDisable_EndToEnd drives config-seeded disabled entries of
// all three kinds through the full running gateway (real JWKS, real SQLite,
// a real downstream MCP subprocess) and asserts the #95 semantics.
func TestApp_EnableDisable_EndToEnd(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	jwks := newJWKSTestServer(t, &key.PublicKey)
	defer jwks.Close()

	require.NoError(t, e2eConfig(t, jwks.URL).Validate())
	dataAddr, _, cleanup := startTestApp(t, e2eConfig(t, jwks.URL))
	defer cleanup()

	tokenFor := func(role string) string {
		now := time.Now()
		return signToken(t, key, jwt.MapClaims{
			"iss": testIssuer, "aud": testAudience, "sub": "u", "role": role,
			"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
		})
	}

	call := func(t *testing.T, token, mcp, tool string) (*http.Response, map[string]any) {
		t.Helper()
		body, err := json.Marshal(map[string]any{"mcp": mcp, "tool": tool, "arguments": map[string]any{"message": echoedMessage}})
		require.NoError(t, err)
		req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s/v1/call", dataAddr), bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		raw, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		_ = resp.Body.Close()
		var decoded map[string]any
		_ = json.Unmarshal(raw, &decoded)
		return resp, decoded
	}

	t.Run("disabled MCP rejects every call with 403 mcp_disabled", func(t *testing.T) {
		resp, body := call(t, tokenFor("user"), "mcp-off", "echo")
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
		assert.Equal(t, "mcp_disabled", body["error"])
	})

	t.Run("disabled access policy grants nothing: 403 forbidden, pipeline never starts", func(t *testing.T) {
		resp, body := call(t, tokenFor("ghost"), "mcp-live", "echo")
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
		assert.Equal(t, "forbidden", body["error"])
	})

	t.Run("disabled filter policy strips nothing; the enabled one still does", func(t *testing.T) {
		resp, body := call(t, tokenFor("user"), "mcp-live", "echo")
		require.Equal(t, http.StatusOK, resp.StatusCode)

		structured, ok := body["structuredContent"].(map[string]any)
		require.True(t, ok, "response has structured content: %v", body)
		assert.Equal(t, "s3cr3t", structured["secret"], "filter-off is disabled, so $.secret is not stripped")
		_, hasMessage := structured["message"]
		assert.False(t, hasMessage, "filter-live is enabled, so $.message is stripped")

		// The payload also travels as a serialized JSON string inside the
		// content block (the MCP spec's own recommendation, which the Go
		// SDK follows). Asserting only on structuredContent is what let
		// #159 ship: the "stripped" field survived here verbatim.
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		assert.NotContains(t, string(raw), echoedMessage,
			"the stripped field must be gone from every copy in the envelope, not just structuredContent")
		assert.Contains(t, string(raw), "s3cr3t", "the field nobody filtered is still there")
	})
}
