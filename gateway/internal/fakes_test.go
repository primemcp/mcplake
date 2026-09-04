package internal_test

import (
	"context"
	"encoding/json"

	"github.com/atsokha/mcplake/auth"
	"github.com/atsokha/mcplake/cache"
	"github.com/atsokha/mcplake/mcp"
)

// fakeAuthenticator is a test double for internal.Authenticator.
type fakeAuthenticator struct {
	claims *auth.Claims
	err    error
}

func (f *fakeAuthenticator) ValidateToken(context.Context, string) (*auth.Claims, error) {
	return f.claims, f.err
}

// fakePolicyEngine is a test double for internal.PolicyEngine.
type fakePolicyEngine struct {
	authorized   bool
	authorizeErr error
	dropFields   []string
	fieldsErr    error
}

func (f *fakePolicyEngine) Authorize(json.RawMessage, string, string) (bool, error) {
	return f.authorized, f.authorizeErr
}

func (f *fakePolicyEngine) FieldsToRemove(json.RawMessage, string, string) ([]string, error) {
	return f.dropFields, f.fieldsErr
}

// fakeResolver is a test double for internal.MCPResolver.
type fakeResolver struct {
	clients map[string]cache.MCPClient
	tools   map[string]map[string]bool
}

func newFakeResolver() *fakeResolver {
	return &fakeResolver{
		clients: make(map[string]cache.MCPClient),
		tools:   make(map[string]map[string]bool),
	}
}

func (f *fakeResolver) addTool(mcpName, tool string, client cache.MCPClient) {
	f.clients[mcpName] = client
	if f.tools[mcpName] == nil {
		f.tools[mcpName] = make(map[string]bool)
	}
	f.tools[mcpName][tool] = true
}

func (f *fakeResolver) Resolve(mcpName string) (cache.MCPClient, bool) {
	c, ok := f.clients[mcpName]
	return c, ok
}

func (f *fakeResolver) HasTool(mcpName, tool string) bool {
	return f.tools[mcpName][tool]
}

// fakeMCPClient is a test double for cache.MCPClient (used as what Resolve
// returns).
type fakeMCPClient struct {
	callTool func(ctx context.Context, tool string, args map[string]any) (*mcp.ToolResponse, error)
}

func (f *fakeMCPClient) ListTools(context.Context) ([]mcp.ToolSchema, error) {
	return nil, nil
}

func (f *fakeMCPClient) CallTool(ctx context.Context, tool string, args map[string]any) (*mcp.ToolResponse, error) {
	return f.callTool(ctx, tool, args)
}

func (f *fakeMCPClient) Close() error { return nil }
