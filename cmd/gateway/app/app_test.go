package app_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/MicahParks/jwkset"
	"github.com/golang-jwt/jwt/v5"
	"github.com/primemcp/mcplake/cmd/gateway/app"
	"github.com/primemcp/mcplake/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testIssuer   = "https://issuer.example.test"
	testAudience = "mcp-gateway-test"
	testKID      = "test-key"
)

// newJWKSTestServer starts an HTTP server serving pub as a JWK Set under
// testKID, mirroring the auth package's own test setup (auth_test.go is
// unexported, so this end-to-end test builds its own minimal equivalent).
func newJWKSTestServer(t *testing.T, pub *rsa.PublicKey) *httptest.Server {
	t.Helper()
	jwk, err := jwkset.NewJWKFromKey(pub, jwkset.JWKOptions{
		Metadata: jwkset.JWKMetadataOptions{KID: testKID, ALG: jwkset.AlgRS256},
	})
	require.NoError(t, err)

	body, err := json.Marshal(jwkset.JWKSMarshal{Keys: []jwkset.JWKMarshal{jwk.Marshal()}})
	require.NoError(t, err)

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
}

func signToken(t *testing.T, key *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = testKID
	signed, err := token.SignedString(key)
	require.NoError(t, err)
	return signed
}

func testConfig(t *testing.T, jwksURL string) *config.Config {
	t.Helper()
	return &config.Config{
		Server: config.ServerConfig{
			DataPlaneAddr:    "127.0.0.1:0",
			ControlPlaneAddr: "127.0.0.1:0",
		},
		OIDC: config.OIDCConfig{
			JWKSURL:  jwksURL,
			Issuer:   testIssuer,
			Audience: testAudience,
		},
		Persistence: config.PersistenceConfig{
			DSN: filepath.Join(t.TempDir(), "test.db"),
		},
	}
}

func TestNew_WiresBothSurfacesWithoutStarting(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	jwks := newJWKSTestServer(t, &key.PublicKey)
	defer jwks.Close()

	a, err := app.New(context.Background(), testConfig(t, jwks.URL))

	require.NoError(t, err)
	require.NotNil(t, a.Gateway)
	require.NotNil(t, a.ControlPlane)
	assert.Empty(t, a.Gateway.Addr())
	assert.Empty(t, a.ControlPlane.Addr())
}

func TestNew_FailsFastOnUnreachableJWKS(t *testing.T) {
	cfg := testConfig(t, "http://127.0.0.1:1/does-not-exist")

	_, err := app.New(context.Background(), cfg)

	require.Error(t, err)
}

// startTestApp builds and runs an App on ephemeral ports, returning its
// bound addresses and a cleanup func that cancels the run context and waits
// for Run to return.
func startTestApp(t *testing.T, cfg *config.Config) (dataPlaneAddr, controlPlaneAddr string, cleanup func()) {
	t.Helper()
	a, err := app.New(context.Background(), cfg)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() { runErr <- a.Run(ctx) }()

	select {
	case <-a.Gateway.Ready():
	case err := <-runErr:
		t.Fatalf("app failed to start: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for data-plane readiness")
	}
	select {
	case <-a.ControlPlane.Ready():
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for control-plane readiness")
	}

	cleanup = func() {
		cancel()
		select {
		case err := <-runErr:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("Run did not return after context cancellation")
		}
	}
	return a.Gateway.Addr(), a.ControlPlane.Addr(), cleanup
}

func TestApp_RunServesBothHealthzEndpoints(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	jwks := newJWKSTestServer(t, &key.PublicKey)
	defer jwks.Close()

	dataAddr, controlAddr, cleanup := startTestApp(t, testConfig(t, jwks.URL))
	defer cleanup()

	resp, err := http.Get(fmt.Sprintf("http://%s/healthz", dataAddr))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	resp2, err := http.Get(fmt.Sprintf("http://%s/admin/healthz", controlAddr))
	require.NoError(t, err)
	defer resp2.Body.Close()
	assert.Equal(t, http.StatusOK, resp2.StatusCode)
}

// TestApp_AdminAccessPolicyChangeIsImmediatelyVisibleOnDataPlane is the
// epic-level integration test ticket #51 calls for: a policy created via
// the admin API changes POST /v1/call's authorize behavior without a
// restart. It uses a genuinely signed+validated JWT (real auth.Validator
// against a local JWKS server) and genuine persistence (temp-file SQLite).
//
// It stops short of a fully successful tool call (200 with a filtered
// body): that would need a live downstream MCP subprocess, and
// Registry.Register's connect/discover/call behavior is already covered by
// the cache and mcp packages' own test suites (Epic #2). What's new here —
// and what this test proves — is that App's wiring makes an admin-API
// write reach the same PolicyStore the data plane's Authorize call reads,
// live: 403 (no grant) before creating the policy, 404 (authorized, but no
// such MCP registered) after.
func TestApp_AdminAccessPolicyChangeIsImmediatelyVisibleOnDataPlane(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	jwks := newJWKSTestServer(t, &key.PublicKey)
	defer jwks.Close()

	dataAddr, controlAddr, cleanup := startTestApp(t, testConfig(t, jwks.URL))
	defer cleanup()

	now := time.Now()
	token := signToken(t, key, jwt.MapClaims{
		"iss":  testIssuer,
		"aud":  testAudience,
		"sub":  "user-1",
		"role": "db-reader",
		"iat":  now.Unix(),
		"exp":  now.Add(time.Hour).Unix(),
	})

	callBody, err := json.Marshal(map[string]any{"mcp": "postgres-ro", "tool": "get_user"})
	require.NoError(t, err)

	doCall := func() *http.Response {
		req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s/v1/call", dataAddr), bytes.NewReader(callBody))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		return resp
	}

	// Before any access policy exists: authorization itself fails (403).
	before := doCall()
	defer before.Body.Close()
	var beforeBody map[string]string
	require.NoError(t, json.NewDecoder(before.Body).Decode(&beforeBody))
	assert.Equal(t, http.StatusForbidden, before.StatusCode)
	assert.Equal(t, "forbidden", beforeBody["error"])

	// Create an access policy via the admin API granting db-reader -> (postgres-ro, get_user).
	policyBody, err := json.Marshal(map[string]any{
		"name":   "db-reader",
		"match":  []map[string]any{{"path": "$.role", "pattern": "^db-reader$"}},
		"grants": []map[string]any{{"mcp": "postgres-ro", "tools": []string{"get_user"}}},
	})
	require.NoError(t, err)
	createResp, err := http.Post(
		fmt.Sprintf("http://%s/admin/access-policies", controlAddr),
		"application/json",
		bytes.NewReader(policyBody),
	)
	require.NoError(t, err)
	defer createResp.Body.Close()
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	// After: authorization now succeeds; the call fails later (no such MCP
	// registered), proving Authorize's decision — not the rest of the
	// pipeline — is what changed.
	after := doCall()
	defer after.Body.Close()
	var afterBody map[string]string
	require.NoError(t, json.NewDecoder(after.Body).Decode(&afterBody))
	assert.Equal(t, http.StatusNotFound, after.StatusCode)
	assert.Equal(t, "mcp_not_found", afterBody["error"])
}
