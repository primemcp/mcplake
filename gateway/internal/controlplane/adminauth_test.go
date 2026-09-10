package controlplane_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/atsokha/mcplake/auth"
	"github.com/atsokha/mcplake/internal/controlplane"
	"github.com/atsokha/mcplake/router"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeValidator is a controlplane token-validator stand-in: it maps known
// tokens to claim documents and rejects everything else.
type fakeValidator struct {
	tokens map[string]string // token -> raw claims JSON
}

func (f fakeValidator) ValidateToken(_ context.Context, token string) (*auth.Claims, error) {
	raw, ok := f.tokens[token]
	if !ok {
		return nil, errors.New("auth: unauthorized")
	}
	return &auth.Claims{Raw: json.RawMessage(raw)}, nil
}

func adminMatcher(t *testing.T) router.ClaimMatcher {
	t.Helper()
	rule, err := router.NewClaimRule("$.role", "^admin$")
	require.NoError(t, err)
	return router.ClaimMatcher{Rules: []router.ClaimRule{rule}}
}

func TestAdminAuth_MissingHeaderIsRejected(t *testing.T) {
	rec := doGuardedRequest(t, "/admin/ping", "")

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, "missing_authorization", errorCode(t, rec.Body.Bytes()))
}

func TestAdminAuth_NonBearerHeaderIsRejected(t *testing.T) {
	rec := doGuardedRequest(t, "/admin/ping", "Basic abc123")

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, "invalid_authorization", errorCode(t, rec.Body.Bytes()))
}

func TestAdminAuth_EmptyBearerTokenIsRejected(t *testing.T) {
	rec := doGuardedRequest(t, "/admin/ping", "Bearer    ")

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, "invalid_authorization", errorCode(t, rec.Body.Bytes()))
}

func TestAdminAuth_InvalidTokenIsRejected(t *testing.T) {
	rec := doGuardedRequest(t, "/admin/ping", "Bearer not-a-known-token")

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, "unauthorized", errorCode(t, rec.Body.Bytes()))
}

func TestAdminAuth_ValidTokenFailingClaimRulesIsForbidden(t *testing.T) {
	rec := doGuardedRequest(t, "/admin/ping", "Bearer db-reader-token")

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Equal(t, "forbidden", errorCode(t, rec.Body.Bytes()))
}

func TestAdminAuth_ValidAdminTokenPassesThrough(t *testing.T) {
	rec := doGuardedRequest(t, "/admin/ping", "Bearer admin-token")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "pong", rec.Body.String())
}

func TestAdminAuth_HealthzStaysOpen(t *testing.T) {
	rec := doGuardedRequest(t, "/admin/healthz", "")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "ok", rec.Body.String())
}

// doGuardedRequest builds a Server with AdminAuth installed and a dummy
// GET /admin/ping on the guarded group, then serves one request with the
// given Authorization header value (empty = no header).
func doGuardedRequest(t *testing.T, path, authHeader string) *httptest.ResponseRecorder {
	t.Helper()

	v := fakeValidator{tokens: map[string]string{
		"admin-token":     `{"role":"admin"}`,
		"db-reader-token": `{"role":"db-reader"}`,
	}}

	s := controlplane.NewServer(controlplane.Config{
		ControlPlaneAddr: "127.0.0.1:0",
		AdminAuth:        controlplane.AdminAuth(v, adminMatcher(t)),
	})
	s.Admin().GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })

	req := httptest.NewRequest(http.MethodGet, path, nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rec := httptest.NewRecorder()
	s.Engine().ServeHTTP(rec, req)
	return rec
}

func errorCode(t *testing.T, body []byte) string {
	t.Helper()
	var e struct {
		Error string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(body, &e))
	return e.Error
}
