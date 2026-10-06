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
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/primemcp/mcplake/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// employeeRecord carries one field a filter policy is told to drop and one
// it is not, so a passing assertion has to distinguish them rather than
// just observing that "something was removed".
type employeeRecord struct {
	Name      string `json:"name"`
	SalaryUSD int    `json:"salary_usd"`
}

// startRemoteMCP runs a one-tool MCP server behind an httptest server,
// speaking Streamable HTTP -- the ADR-0017 shape: a server the gateway
// neither starts nor shares a process with. httptest binds 127.0.0.1, so
// the URL is loopback and satisfies mcp.ValidateEndpointURL without the
// test relaxing anything.
func startRemoteMCP(t *testing.T) string {
	t.Helper()
	server := sdk.NewServer(&sdk.Implementation{Name: "mcplake-e2e-remote", Version: "0.1.0"}, nil)
	sdk.AddTool(server, &sdk.Tool{
		Name:        "get_employee",
		Description: "returns one employee record",
	}, func(_ context.Context, _ *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, employeeRecord, error) {
		return nil, employeeRecord{Name: "Marcus Webb", SalaryUSD: 198000}, nil
	})

	handler := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, nil)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv.URL
}

// The acceptance criterion for ADR-0017, end to end through the real
// binary's wiring: an MCP the gateway does not run, registered from config
// over http, is discovered, called, authorized and filtered exactly as a
// stdio one would be.
//
// This is also the suite's first fully successful POST /v1/call -- a 200
// with a filtered body. The older tests stop at 404 ("authorized, but no
// such MCP") because a live downstream previously meant managing a
// subprocess; an http downstream is just an httptest server, so the last
// step of the pipeline is finally cheap to assert.
func TestApp_HTTPTransportMCP_EndToEnd(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	jwks := newJWKSTestServer(t, &key.PublicKey)
	defer jwks.Close()

	cfg := testConfig(t, jwks.URL)
	cfg.MCPs = []config.MCPConfig{{
		Name: "remote-directory",
		Type: "http",
		URL:  startRemoteMCP(t),
	}}
	cfg.AccessPolicies = []config.AccessPolicyConfig{{
		Name:   "db-reader",
		Match:  []config.ClaimRuleConfig{{Path: "$.role", Pattern: "^db-reader$"}},
		Grants: []config.GrantConfig{{MCP: "remote-directory", Tools: []string{"*"}}},
	}}
	cfg.FilterPolicies = []config.FilterPolicyConfig{{
		Name:       "hide-salary",
		Match:      []config.ClaimRuleConfig{{Path: "$.role", Pattern: "^db-reader$"}},
		MCP:        "remote-directory",
		Tool:       "get_employee",
		DropFields: []string{"$.salary_usd"},
	}}

	dataAddr, _, cleanup := startTestApp(t, cfg)
	defer cleanup()

	now := time.Now()
	token := signToken(t, key, jwt.MapClaims{
		"iss":  testIssuer,
		"aud":  testAudience,
		"sub":  "alice",
		"role": "db-reader",
		"iat":  now.Unix(),
		"exp":  now.Add(time.Hour).Unix(),
	})

	callBody, err := json.Marshal(map[string]any{"mcp": "remote-directory", "tool": "get_employee"})
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s/v1/call", dataAddr), bytes.NewReader(callBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	assert.Contains(t, string(body), "Marcus Webb", "the remote MCP's response should come through")
	assert.NotContains(t, string(body), "salary_usd",
		"the filter policy must apply to an http MCP exactly as it does to a stdio one")
	assert.NotContains(t, string(body), "198000")
}

// The other half of the same wiring: authorization is not bypassed just
// because the MCP is remote.
func TestApp_HTTPTransportMCP_StillEnforcesAccessPolicies(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	jwks := newJWKSTestServer(t, &key.PublicKey)
	defer jwks.Close()

	cfg := testConfig(t, jwks.URL)
	cfg.MCPs = []config.MCPConfig{{
		Name: "remote-directory",
		Type: "http",
		URL:  startRemoteMCP(t),
	}}

	dataAddr, _, cleanup := startTestApp(t, cfg)
	defer cleanup()

	now := time.Now()
	token := signToken(t, key, jwt.MapClaims{
		"iss":  testIssuer,
		"aud":  testAudience,
		"sub":  "bob",
		"role": "guest",
		"iat":  now.Unix(),
		"exp":  now.Add(time.Hour).Unix(),
	})

	callBody, err := json.Marshal(map[string]any{"mcp": "remote-directory", "tool": "get_employee"})
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s/v1/call", dataAddr), bytes.NewReader(callBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}
