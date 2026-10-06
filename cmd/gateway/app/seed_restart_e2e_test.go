package app_test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/primemcp/mcplake/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedRestartConfig declares one MCP and one access policy in the config
// file -- the shape that made a restart revert operator actions before
// ADR-0016. Both listeners and the store are fixed per test so two
// consecutive runs share the same database, which is what a restart is.
func seedRestartConfig(t *testing.T, jwksURL, dsn string) *config.Config {
	t.Helper()
	echoCmd := os.Args[0]
	echoArgs := []string{"-test.run=^TestHelperEchoMCPProcess$", "--", echoMCPMarker}

	return &config.Config{
		Server: config.ServerConfig{DataPlaneAddr: "127.0.0.1:0", ControlPlaneAddr: "127.0.0.1:0"},
		OIDC:   config.OIDCConfig{JWKSURL: jwksURL, Issuer: testIssuer, Audience: testAudience},
		Persistence: config.PersistenceConfig{
			DSN: dsn,
		},
		MCPs: []config.MCPConfig{
			// No `enabled` key: an omitted value means enabled, which is
			// exactly what used to overwrite a runtime disable.
			{Name: "seeded-mcp", Command: echoCmd, Arguments: echoArgs},
		},
		AccessPolicies: []config.AccessPolicyConfig{
			{
				Name:   "seeded-policy",
				Match:  []config.ClaimRuleConfig{{Path: "$.role", Pattern: "^user$"}},
				Grants: []config.GrantConfig{{MCP: "seeded-mcp", Tools: []string{"*"}}},
			},
		},
	}
}

func adminGET(t *testing.T, addr, path string) []map[string]any {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("http://%s%s", addr, path))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var decoded []map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	return decoded
}

func adminSend(t *testing.T, method, url string, body any) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, url, reader)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

// TestApp_SeedDoesNotRevertRuntimeChangesAcrossRestart drives the whole
// gateway twice against one store: disable an MCP and delete a policy
// through the admin API, restart, and require both to still be gone.
// Before ADR-0016 the config file was re-applied on every boot, so the MCP
// came back enabled and the policy came back entirely -- silently, with the
// docs promising the opposite.
func TestApp_SeedDoesNotRevertRuntimeChangesAcrossRestart(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	jwks := newJWKSTestServer(t, &key.PublicKey)
	defer jwks.Close()

	dsn := filepath.Join(t.TempDir(), "restart.db")

	// --- first boot: config seeds both entries ---
	_, controlAddr, cleanup := startTestApp(t, seedRestartConfig(t, jwks.URL, dsn))

	mcps := adminGET(t, controlAddr, "/admin/mcps")
	require.Len(t, mcps, 1)
	require.Equal(t, true, mcps[0]["enabled"], "a seeded MCP starts enabled")
	require.Len(t, adminGET(t, controlAddr, "/admin/access-policies"), 1)

	// --- operator revokes at runtime ---
	resp := adminSend(t, http.MethodPatch, fmt.Sprintf("http://%s/admin/mcps/seeded-mcp", controlAddr), map[string]any{"enabled": false})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	resp = adminSend(t, http.MethodDelete, fmt.Sprintf("http://%s/admin/access-policies/seeded-policy", controlAddr), nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	cleanup()

	// --- restart against the same store ---
	_, controlAddr, cleanup = startTestApp(t, seedRestartConfig(t, jwks.URL, dsn))
	defer cleanup()

	mcps = adminGET(t, controlAddr, "/admin/mcps")
	require.Len(t, mcps, 1, "the MCP is still registered, just disabled")
	assert.Equal(t, false, mcps[0]["enabled"], "a disable that does not survive a restart is not a disable")

	assert.Empty(t, adminGET(t, controlAddr, "/admin/access-policies"),
		"a policy deleted through the admin API must not be resurrected by seeding")
}

// An entry added to the config file after the first boot is still seeded:
// the contract is "once per entry", not "once ever".
func TestApp_SeedStillAppliesAConfigEntryAddedLater(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	jwks := newJWKSTestServer(t, &key.PublicKey)
	defer jwks.Close()

	dsn := filepath.Join(t.TempDir(), "restart.db")
	_, _, cleanup := startTestApp(t, seedRestartConfig(t, jwks.URL, dsn))
	cleanup()

	cfg := seedRestartConfig(t, jwks.URL, dsn)
	cfg.AccessPolicies = append(cfg.AccessPolicies, config.AccessPolicyConfig{
		Name:   "added-later",
		Match:  []config.ClaimRuleConfig{{Path: "$.role", Pattern: "^auditor$"}},
		Grants: []config.GrantConfig{{MCP: "seeded-mcp", Tools: []string{"*"}}},
	})

	_, controlAddr, cleanup := startTestApp(t, cfg)
	defer cleanup()

	names := make([]string, 0, 2)
	for _, p := range adminGET(t, controlAddr, "/admin/access-policies") {
		names = append(names, p["name"].(string))
	}
	assert.Contains(t, names, "added-later")
}
