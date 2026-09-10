package app_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestApp_AdminAuth_EndToEnd boots the fully wired gateway with a real
// local JWKS, real SQLite, and an admin_auth.match rule requiring
// `role` == "admin", then exercises every admin-auth outcome against the
// live control-plane listener.
func TestApp_AdminAuth_EndToEnd(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	jwks := newJWKSTestServer(t, &key.PublicKey)
	defer jwks.Close()

	_, controlAddr, cleanup := startTestApp(t, adminAuthConfig(t, jwks.URL))
	defer cleanup()

	mcpsURL := fmt.Sprintf("http://%s/admin/mcps", controlAddr)
	get := func(authHeader string) (*http.Response, map[string]any) {
		req, err := http.NewRequest(http.MethodGet, mcpsURL, nil)
		require.NoError(t, err)
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		resp.Body.Close()

		var decoded map[string]any
		_ = json.Unmarshal(body, &decoded)
		return resp, decoded
	}

	claims := func(role string) string {
		now := time.Now()
		return signToken(t, key, jwt.MapClaims{
			"iss": testIssuer, "aud": testAudience, "sub": "u", "role": role,
			"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
		})
	}

	t.Run("no Authorization header -> 401 missing_authorization", func(t *testing.T) {
		resp, body := get("")
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.Equal(t, "missing_authorization", body["error"])
	})

	t.Run("non-Bearer header -> 401 invalid_authorization", func(t *testing.T) {
		resp, body := get("Basic Zm9vOmJhcg==")
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.Equal(t, "invalid_authorization", body["error"])
	})

	t.Run("garbage bearer token -> 401 unauthorized", func(t *testing.T) {
		resp, body := get("Bearer not.a.jwt")
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.Equal(t, "unauthorized", body["error"])
	})

	t.Run("valid token without the admin claim -> 403 forbidden", func(t *testing.T) {
		resp, body := get("Bearer " + claims("db-reader"))
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
		assert.Equal(t, "forbidden", body["error"])
	})

	t.Run("valid admin token -> 200 with the (empty) MCP list", func(t *testing.T) {
		resp, _ := get("Bearer " + claims("admin"))
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("healthz is reachable with no token", func(t *testing.T) {
		resp, err := http.Get(fmt.Sprintf("http://%s/admin/healthz", controlAddr))
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}
