package app_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/atsokha/mcplake/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getAuthConfigJSON calls the endpoint the admin UI bootstraps from, with
// no credentials at all -- which is the entire point of it (ADR-0014).
func getAuthConfigJSON(t *testing.T, controlAddr string) map[string]any {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("http://%s/admin/auth/config", controlAddr))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	decoded := map[string]any{}
	require.NoError(t, json.Unmarshal(body, &decoded))
	return decoded
}

func TestApp_AdminLogin_ServesLoginCoordinatesWithoutAToken(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	jwks := newJWKSTestServer(t, &key.PublicKey)
	defer jwks.Close()

	cfg := adminAuthConfig(t, jwks.URL)
	cfg.AdminAuth.Login = config.AdminLoginConfig{
		ClientID:              "mcplake-admin-ui",
		AuthorizationEndpoint: "https://auth.example.com/authorize",
		TokenEndpoint:         "https://auth.example.com/oauth/token",
	}

	_, controlAddr, cleanup := startTestApp(t, cfg)
	defer cleanup()

	body := getAuthConfigJSON(t, controlAddr)

	assert.Equal(t, true, body["auth_required"])
	assert.Equal(t, "mcplake-admin-ui", body["client_id"])
	assert.Equal(t, "https://auth.example.com/authorize", body["authorization_endpoint"])
	assert.Equal(t, "https://auth.example.com/oauth/token", body["token_endpoint"])
	assert.Equal(t, testIssuer, body["issuer"])
	// Defaulted, not echoed from config -- the UI must never have to guess.
	assert.Equal(t, []any{"openid", "profile", "email"}, body["scopes"])
}

// Admin auth on, no [admin_auth.login]: the UI has to be able to tell this
// apart from "log in like this" and say so, rather than redirect nowhere.
func TestApp_AdminLogin_ReportsAuthRequiredWithNoCoordinatesWhenLoginIsUnset(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	jwks := newJWKSTestServer(t, &key.PublicKey)
	defer jwks.Close()

	_, controlAddr, cleanup := startTestApp(t, adminAuthConfig(t, jwks.URL))
	defer cleanup()

	body := getAuthConfigJSON(t, controlAddr)

	assert.Equal(t, true, body["auth_required"])
	assert.NotContains(t, body, "client_id")
	assert.NotContains(t, body, "authorization_endpoint")
	assert.NotContains(t, body, "token_endpoint")
}

// With the gate off the UI must behave exactly as it did before ADR-0010:
// no login screen, no Authorization header, nothing to configure.
func TestApp_AdminLogin_ReportsNoAuthRequiredWhenTheGateIsOff(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	jwks := newJWKSTestServer(t, &key.PublicKey)
	defer jwks.Close()

	_, controlAddr, cleanup := startTestApp(t, testConfig(t, jwks.URL))
	defer cleanup()

	body := getAuthConfigJSON(t, controlAddr)

	assert.Equal(t, false, body["auth_required"])
	assert.NotContains(t, body, "client_id")
}
