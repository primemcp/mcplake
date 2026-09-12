package controlplane_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/atsokha/mcplake/internal/controlplane"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getAuthConfig(t *testing.T, cfg controlplane.Config) (int, map[string]any) {
	t.Helper()
	s := controlplane.NewServer(cfg)
	req := httptest.NewRequest(http.MethodGet, "/admin/auth/config", nil)
	rec := httptest.NewRecorder()

	s.Engine().ServeHTTP(rec, req)

	body := map[string]any{}
	if rec.Body.Len() > 0 {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	}
	return rec.Code, body
}

func loginInfo() controlplane.AuthInfo {
	return controlplane.AuthInfo{
		AuthRequired:          true,
		Issuer:                "https://auth.example.com",
		ClientID:              "mcplake-admin-ui",
		AuthorizationEndpoint: "https://auth.example.com/authorize",
		TokenEndpoint:         "https://auth.example.com/oauth/token",
		Scopes:                []string{"openid", "profile", "email"},
	}
}

// The whole point of this endpoint: a browser that has no token yet must be
// able to learn how to get one. It therefore sits ahead of the admin-auth
// middleware, like GET /admin/healthz (ADR-0014).
func TestAuthConfig_IsReachableWithoutATokenWhileAdminAuthIsOn(t *testing.T) {
	code, body := getAuthConfig(t, controlplane.Config{
		AdminAuth: controlplane.AdminAuth(fakeValidator{}, adminMatcher(t)),
		AuthInfo:  loginInfo(),
	})

	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, body["auth_required"])
	assert.Equal(t, "mcplake-admin-ui", body["client_id"])
	assert.Equal(t, "https://auth.example.com", body["issuer"])
	assert.Equal(t, "https://auth.example.com/authorize", body["authorization_endpoint"])
	assert.Equal(t, "https://auth.example.com/oauth/token", body["token_endpoint"])
	assert.Equal(t, []any{"openid", "profile", "email"}, body["scopes"])
}

func TestAuthConfig_ReportsAuthNotRequiredWhenTheGateIsOff(t *testing.T) {
	code, body := getAuthConfig(t, controlplane.Config{})

	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, false, body["auth_required"])
}

// Admin auth on but no [admin_auth.login]: a real, reportable state (the UI
// says so instead of offering a broken sign-in button), not an error.
func TestAuthConfig_AuthRequiredWithoutLoginCoordinates(t *testing.T) {
	code, body := getAuthConfig(t, controlplane.Config{
		AdminAuth: controlplane.AdminAuth(fakeValidator{}, adminMatcher(t)),
		AuthInfo:  controlplane.AuthInfo{AuthRequired: true},
	})

	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, body["auth_required"])
	assert.NotContains(t, body, "client_id")
	assert.NotContains(t, body, "authorization_endpoint")
	assert.NotContains(t, body, "token_endpoint")
}

// Guards the security claim in ADR-0014: what this open endpoint discloses
// is exactly the public half of a PKCE client, nothing else. A new field on
// AuthInfo that isn't safe to expose should fail here.
func TestAuthConfig_DisclosesNothingBeyondThePublicLoginCoordinates(t *testing.T) {
	_, body := getAuthConfig(t, controlplane.Config{
		AdminAuth: controlplane.AdminAuth(fakeValidator{}, adminMatcher(t)),
		AuthInfo:  loginInfo(),
	})

	keys := make([]string, 0, len(body))
	for k := range body {
		keys = append(keys, k)
	}
	assert.ElementsMatch(t, []string{
		"auth_required", "issuer", "client_id", "authorization_endpoint", "token_endpoint", "scopes",
	}, keys)
}

// Everything else under /admin stays gated -- adding an open route must not
// have opened the group.
func TestAuthConfig_DoesNotOpenTheRestOfAdmin(t *testing.T) {
	s := controlplane.NewServer(controlplane.Config{
		AdminAuth: controlplane.AdminAuth(fakeValidator{}, adminMatcher(t)),
		AuthInfo:  loginInfo(),
	})
	s.Admin().GET("/guarded", func(c *gin.Context) { c.String(http.StatusOK, "secret") })

	req := httptest.NewRequest(http.MethodGet, "/admin/guarded", nil)
	rec := httptest.NewRecorder()
	s.Engine().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}
