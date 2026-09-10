package controlplane

import (
	"net/http"
	"strings"

	"github.com/atsokha/mcplake/internal/controlplane/adminmcp"
	"github.com/atsokha/mcplake/internal/controlplane/adminservice"
	"github.com/gin-gonic/gin"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterMCPControlServer mounts the MCP control server (ADR-0011) as a
// streamable-HTTP handler on admin at fullPath, which must be under the
// group's "/admin" prefix. Registering it on the admin group means the
// admin-auth middleware NewServer installed runs first: an MCP client has
// to present a valid admin JWT to connect (or, if admin auth is not
// configured, the control server is open — the caller logs that).
//
// The handler is method-agnostic (the streamable transport keys off the
// HTTP method and a session header, not the path), so a single Any route
// covers the GET stream, POST messages, and DELETE teardown.
func RegisterMCPControlServer(admin *gin.RouterGroup, svc *adminservice.Services, fullPath string) {
	server := adminmcp.NewServer(svc)
	handler := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, nil)

	rel := strings.TrimPrefix(fullPath, "/admin")
	if rel == "" || rel == fullPath {
		rel = "/mcp"
	}
	admin.Any(rel, gin.WrapH(handler))
}
