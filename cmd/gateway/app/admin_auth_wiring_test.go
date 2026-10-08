package app_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/primemcp/mcplake/cmd/gateway/app"
	"github.com/primemcp/mcplake/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// adminAuthConfig is testConfig plus an admin_auth.match rule requiring
// `role` == "admin".
func adminAuthConfig(t *testing.T, jwksURL string) *config.Config {
	t.Helper()
	cfg := testConfig(t, jwksURL)
	cfg.AdminAuth = config.AdminAuthConfig{
		Match: []config.ClaimRuleConfig{{Path: "$.role", Pattern: "^admin$"}},
	}
	return cfg
}

func TestApp_AdminAuthWiring_GuardsControlPlaneButNotHealthz(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	jwks := newJWKSTestServer(t, &key.PublicKey)
	defer jwks.Close()

	_, controlAddr, cleanup := startTestApp(t, adminAuthConfig(t, jwks.URL))
	defer cleanup()

	// Health check stays open.
	health, err := http.Get(fmt.Sprintf("http://%s/admin/healthz", controlAddr))
	require.NoError(t, err)
	defer health.Body.Close()
	assert.Equal(t, http.StatusOK, health.StatusCode)

	// A CRUD route with no token is rejected by the middleware.
	noToken, err := http.Get(fmt.Sprintf("http://%s/admin/mcps", controlAddr))
	require.NoError(t, err)
	defer noToken.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, noToken.StatusCode)

	// With an admin-claim token it passes through to the handler.
	now := time.Now()
	token := signToken(t, key, jwt.MapClaims{
		"iss": testIssuer, "aud": testAudience, "sub": "admin-1", "role": "admin",
		"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
	})
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s/admin/mcps", controlAddr), nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	withToken, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer withToken.Body.Close()
	assert.Equal(t, http.StatusOK, withToken.StatusCode)
}

func TestApp_New_WarnsWhenAdminAuthUnconfigured(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	jwks := newJWKSTestServer(t, &key.PublicKey)
	defer jwks.Close()

	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	defer slog.SetDefault(prev)

	_, err = app.New(context.Background(), testConfig(t, jwks.URL))
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "UNAUTHENTICATED")
}

func TestApp_New_NoWarnWhenAdminAuthConfigured(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	jwks := newJWKSTestServer(t, &key.PublicKey)
	defer jwks.Close()

	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	defer slog.SetDefault(prev)

	_, err = app.New(context.Background(), adminAuthConfig(t, jwks.URL))
	require.NoError(t, err)

	assert.NotContains(t, buf.String(), "UNAUTHENTICATED")
}
