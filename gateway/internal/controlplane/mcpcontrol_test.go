package controlplane_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/atsokha/mcplake/internal/controlplane"
	"github.com/atsokha/mcplake/internal/controlplane/adminservice"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type noReloader struct{}

func (noReloader) Refresh(context.Context) error { return nil }

// mcpControlServices assembles an adminservice.Services from the in-memory
// fakes already defined across the controlplane handler tests.
func mcpControlServices() *adminservice.Services {
	return adminservice.New(
		context.Background(), newFakeMCPRegistry(), newFakeMCPRepository(),
		newFakeAccessPolicyRepo(), fakeFilterPolicyRepo{}, noReloader{},
	)
}

func TestRegisterMCPControlServer_RouteIsMountedAndReachable(t *testing.T) {
	s := controlplane.NewServer(controlplane.Config{ControlPlaneAddr: "127.0.0.1:0"})
	controlplane.RegisterMCPControlServer(s.Admin(), mcpControlServices(), "/admin/mcp")

	// A bare POST (no MCP framing) still reaches the streamable handler: it
	// rejects the request, but with something other than 404 — proving the
	// route exists.
	req := httptest.NewRequest(http.MethodPost, "/admin/mcp", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Engine().ServeHTTP(rec, req)

	assert.NotEqual(t, http.StatusNotFound, rec.Code)
}

func TestRegisterMCPControlServer_IsGuardedByAdminAuth(t *testing.T) {
	v := fakeValidator{tokens: map[string]string{"admin-token": `{"role":"admin"}`}}
	s := controlplane.NewServer(controlplane.Config{
		ControlPlaneAddr: "127.0.0.1:0",
		AdminAuth:        controlplane.AdminAuth(v, adminMatcher(t)),
	})
	controlplane.RegisterMCPControlServer(s.Admin(), mcpControlServices(), "/admin/mcp")

	// A real MCP client always declares a JSON body; without it the
	// content-type guard would reject the request before admin auth ever
	// runs, which is deliberate but not what this test is about.
	req := httptest.NewRequest(http.MethodPost, "/admin/mcp", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Engine().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, "missing_authorization", errorCode(t, rec.Body.Bytes()))
}

func TestRegisterMCPControlServer_CustomPathUnderAdmin(t *testing.T) {
	s := controlplane.NewServer(controlplane.Config{ControlPlaneAddr: "127.0.0.1:0"})
	controlplane.RegisterMCPControlServer(s.Admin(), mcpControlServices(), "/admin/control/mcp")

	req := httptest.NewRequest(http.MethodPost, "/admin/control/mcp", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Engine().ServeHTTP(rec, req)

	require.NotEqual(t, http.StatusNotFound, rec.Code)
}
