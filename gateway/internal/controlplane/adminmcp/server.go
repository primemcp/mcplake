// Package adminmcp exposes the control-plane admin operations as MCP tools,
// so an MCP client can administer a running gateway the same way it calls
// any other tool. It is a thin adapter over
// gateway/internal/controlplane/adminservice — the same application layer
// the Gin REST handlers use, so the two surfaces cannot drift. See
// docs/architecture/decisions/0011-mcp-control-server.md.
package adminmcp

import (
	"encoding/json"

	"github.com/atsokha/mcplake/internal/controlplane/adminservice"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// serverName / serverVersion identify this MCP server to clients in the
// initialize handshake.
const (
	serverName    = "mcplake-admin"
	serverVersion = "0.1.0"
)

// NewServer builds an MCP server that exposes every control-plane admin
// operation as one tool, backed by svc. A tool-level failure (unknown name,
// invalid policy, downstream registration failure) is returned as an MCP
// tool error (IsError) carrying the adminservice message, not a
// protocol-level error.
func NewServer(svc *adminservice.Services) *sdk.Server {
	s := sdk.NewServer(&sdk.Implementation{Name: serverName, Version: serverVersion}, nil)
	registerMCPTools(s, svc.MCP)
	registerAccessPolicyTools(s, svc.Access)
	registerFilterPolicyTools(s, svc.Filter)
	registerHealthTool(s)
	return s
}

// okOutput is the result of a tool whose only meaningful outcome is
// success/failure (the delete/unregister tools).
type okOutput struct {
	OK bool `json:"ok" jsonschema:"true when the operation completed"`
}

// nameInput is the shared input shape for the get/delete/unregister tools.
type nameInput struct {
	Name string `json:"name" jsonschema:"name of the MCP or policy to act on"`
}

// rawToMap decodes a JSON object into a map so tool output schemas infer
// cleanly (a json.RawMessage field would infer as a string). A nil or
// non-object payload yields nil.
func rawToMap(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}
