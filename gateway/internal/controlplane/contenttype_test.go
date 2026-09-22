package controlplane_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/atsokha/mcplake/internal/controlplane"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// serverWithProbe returns a control-plane server carrying one route per
// method that records whether the handler actually ran, so a test can tell
// "rejected before the handler" from "handler ran and returned an error".
func serverWithProbe(t *testing.T) (*controlplane.Server, *bool) {
	t.Helper()
	reached := false
	s := controlplane.NewServer(controlplane.Config{})
	handler := func(c *gin.Context) {
		reached = true
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
	s.Admin().POST("/probe", handler)
	s.Admin().PUT("/probe", handler)
	s.Admin().PATCH("/probe", handler)
	s.Admin().DELETE("/probe", handler)
	s.Admin().GET("/probe", handler)
	return s, &reached
}

func doRequest(t *testing.T, s *controlplane.Server, method, path, contentType, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	s.Engine().ServeHTTP(rec, req)
	return rec
}

// The attack this closes: a cross-origin POST with a CORS-simple
// Content-Type needs no preflight, and Gin's ShouldBindJSON parses the body
// as JSON regardless of the declared type. Requiring application/json means
// a browser must preflight, which an unconfigured control plane never
// answers.
func TestControlPlane_RejectsStateChangingRequestsWithoutJSONContentType(t *testing.T) {
	simple := []string{"text/plain", "application/x-www-form-urlencoded", "multipart/form-data; boundary=x", ""}

	for _, contentType := range simple {
		name := contentType
		if name == "" {
			name = "(no Content-Type)"
		}
		t.Run(name, func(t *testing.T) {
			s, reached := serverWithProbe(t)

			rec := doRequest(t, s, http.MethodPost, "/admin/probe", contentType, `{"name":"x"}`)

			assert.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
			assert.False(t, *reached, "the handler must not run for a request the guard rejects")

			var body map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Equal(t, "unsupported_media_type", body["error"])
		})
	}
}

func TestControlPlane_AcceptsJSONContentType(t *testing.T) {
	// Parameters are part of a well-formed header and must not be rejected;
	// the UI and most HTTP clients send a charset.
	for _, contentType := range []string{"application/json", "application/json; charset=utf-8", "APPLICATION/JSON"} {
		t.Run(contentType, func(t *testing.T) {
			s, reached := serverWithProbe(t)

			rec := doRequest(t, s, http.MethodPost, "/admin/probe", contentType, `{"name":"x"}`)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.True(t, *reached)
		})
	}
}

func TestControlPlane_GuardsEveryMethodThatCarriesABody(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch} {
		t.Run(method, func(t *testing.T) {
			s, reached := serverWithProbe(t)

			rec := doRequest(t, s, method, "/admin/probe", "text/plain", `{}`)

			assert.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
			assert.False(t, *reached)
		})
	}
}

// DELETE carries no body in this API, and a cross-origin DELETE already
// requires a preflight the control plane never answers -- so demanding a
// Content-Type there would only break `curl -X DELETE` for no security gain.
func TestControlPlane_DeleteAndGetAreUnaffected(t *testing.T) {
	t.Run("DELETE with no Content-Type", func(t *testing.T) {
		s, reached := serverWithProbe(t)

		rec := doRequest(t, s, http.MethodDelete, "/admin/probe", "", "")

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.True(t, *reached)
	})

	t.Run("GET with no Content-Type", func(t *testing.T) {
		s, reached := serverWithProbe(t)

		rec := doRequest(t, s, http.MethodGet, "/admin/probe", "", "")

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.True(t, *reached)
	})
}

// The open bootstrap routes stay reachable: they are GET, so the guard is a
// no-op for them, but this pins that it stays that way.
func TestControlPlane_OpenRoutesStillReachable(t *testing.T) {
	s := controlplane.NewServer(controlplane.Config{})

	for _, path := range []string{"/admin/healthz", "/admin/auth/config"} {
		rec := doRequest(t, s, http.MethodGet, path, "", "")
		assert.Equal(t, http.StatusOK, rec.Code, path)
	}
}

// The guard runs before authentication, so an unauthenticated cross-origin
// POST is rejected on its media type rather than leaking whether admin auth
// is even configured.
func TestControlPlane_ContentTypeGuardRunsBeforeAdminAuth(t *testing.T) {
	s := controlplane.NewServer(controlplane.Config{
		AdminAuth: controlplane.AdminAuth(fakeValidator{}, adminMatcher(t)),
	})
	s.Admin().POST("/probe", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{}) })

	rec := doRequest(t, s, http.MethodPost, "/admin/probe", "text/plain", `{}`)

	assert.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
}
