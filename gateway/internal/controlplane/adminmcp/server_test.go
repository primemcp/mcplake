package adminmcp_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/atsokha/mcplake/cache"
	"github.com/atsokha/mcplake/internal/controlplane/adminmcp"
	"github.com/atsokha/mcplake/internal/controlplane/adminservice"
	"github.com/atsokha/mcplake/router"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- fakes ---------------------------------------------------------------

type fakeRegistry struct {
	regs map[string]cache.MCPRegistration
}

func newFakeRegistry() *fakeRegistry {
	return &fakeRegistry{regs: map[string]cache.MCPRegistration{}}
}

func (f *fakeRegistry) Register(_ context.Context, reg cache.MCPRegistration) error {
	reg.Status = cache.StatusActive
	reg.Tools = map[string]cache.ToolSchema{
		"get_user": {Name: "get_user", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}
	f.regs[reg.Name] = reg
	return nil
}

func (f *fakeRegistry) Unregister(name string) error {
	if _, ok := f.regs[name]; !ok {
		return assertNotRegistered(name)
	}
	delete(f.regs, name)
	return nil
}

func assertNotRegistered(name string) error { return &notRegisteredError{name} }

type notRegisteredError struct{ name string }

func (e *notRegisteredError) Error() string { return "cache: " + e.name + " is not registered" }

func (f *fakeRegistry) Get(name string) (cache.MCPRegistration, bool) {
	reg, ok := f.regs[name]
	return reg, ok
}

func (f *fakeRegistry) List() []cache.MCPRegistration {
	out := make([]cache.MCPRegistration, 0, len(f.regs))
	for _, r := range f.regs {
		out = append(out, r)
	}
	return out
}

func (f *fakeRegistry) SetEnabled(name string, enabled bool) (cache.MCPRegistration, bool) {
	reg, ok := f.regs[name]
	if !ok {
		return cache.MCPRegistration{}, false
	}
	reg.Enabled = enabled
	f.regs[name] = reg
	return reg, true
}

type fakeMCPRepo struct {
	rows map[string]cache.MCPRegistration
}

func newFakeMCPRepo() *fakeMCPRepo { return &fakeMCPRepo{rows: map[string]cache.MCPRegistration{}} }

func (f *fakeMCPRepo) Upsert(_ context.Context, reg cache.MCPRegistration) error {
	f.rows[reg.Name] = reg
	return nil
}
func (f *fakeMCPRepo) Delete(_ context.Context, name string) error {
	delete(f.rows, name)
	return nil
}

type fakeAccessRepo struct {
	policies map[string]router.AccessPolicy
}

func newFakeAccessRepo() *fakeAccessRepo {
	return &fakeAccessRepo{policies: map[string]router.AccessPolicy{}}
}

func (f *fakeAccessRepo) Upsert(_ context.Context, p router.AccessPolicy) error {
	for _, r := range p.Match.Rules {
		if _, err := router.NewClaimRule(r.Path, r.Pattern); err != nil {
			return err
		}
	}
	f.policies[p.Name] = p
	return nil
}
func (f *fakeAccessRepo) Get(_ context.Context, name string) (router.AccessPolicy, bool, error) {
	p, ok := f.policies[name]
	return p, ok, nil
}
func (f *fakeAccessRepo) List(_ context.Context) ([]router.AccessPolicy, error) {
	out := make([]router.AccessPolicy, 0, len(f.policies))
	for _, p := range f.policies {
		out = append(out, p)
	}
	return out, nil
}
func (f *fakeAccessRepo) Delete(_ context.Context, name string) error {
	delete(f.policies, name)
	return nil
}

type fakeFilterRepo struct {
	policies map[string]router.FilterPolicy
}

func newFakeFilterRepo() *fakeFilterRepo {
	return &fakeFilterRepo{policies: map[string]router.FilterPolicy{}}
}

func (f *fakeFilterRepo) Upsert(_ context.Context, p router.FilterPolicy) error {
	f.policies[p.Name] = p
	return nil
}
func (f *fakeFilterRepo) Get(_ context.Context, name string) (router.FilterPolicy, bool, error) {
	p, ok := f.policies[name]
	return p, ok, nil
}
func (f *fakeFilterRepo) List(_ context.Context) ([]router.FilterPolicy, error) {
	out := make([]router.FilterPolicy, 0, len(f.policies))
	for _, p := range f.policies {
		out = append(out, p)
	}
	return out, nil
}
func (f *fakeFilterRepo) Delete(_ context.Context, name string) error {
	delete(f.policies, name)
	return nil
}

type noopReloader struct{}

func (noopReloader) Refresh(context.Context) error { return nil }

// --- harness -----------------------------------------------------------

// connect builds an adminmcp server over an in-memory transport and returns
// a connected client session.
func connect(t *testing.T) *sdk.ClientSession {
	t.Helper()
	svc := adminservice.New(context.Background(), newFakeRegistry(), newFakeMCPRepo(), newFakeAccessRepo(), newFakeFilterRepo(), noopReloader{})
	server := adminmcp.NewServer(svc)

	serverT, clientT := sdk.NewInMemoryTransports()
	ctx := context.Background()
	serverSession, err := server.Connect(ctx, serverT, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = serverSession.Close() })

	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "0"}, nil)
	clientSession, err := client.Connect(ctx, clientT, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = clientSession.Close() })
	return clientSession
}

func call(t *testing.T, cs *sdk.ClientSession, name string, args any) *sdk.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	return res
}

func decode(t *testing.T, res *sdk.CallToolResult, target any) {
	t.Helper()
	raw, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, target))
}

// --- tests -----------------------------------------------------------

func TestNewServer_RegistersEveryTool(t *testing.T) {
	cs := connect(t)

	got, err := cs.ListTools(context.Background(), nil)
	require.NoError(t, err)

	names := make(map[string]bool)
	for _, tool := range got.Tools {
		names[tool.Name] = true
		assert.NotNil(t, tool.InputSchema, "tool %q has no input schema", tool.Name)
	}

	for _, want := range []string{
		"list_mcps", "register_mcp", "set_mcp_enabled", "unregister_mcp",
		"list_access_policies", "get_access_policy", "create_access_policy",
		"replace_access_policy", "delete_access_policy",
		"list_filter_policies", "get_filter_policy", "create_filter_policy",
		"replace_filter_policy", "delete_filter_policy",
		"gateway_health",
	} {
		assert.Contains(t, names, want)
	}
}

func TestMCPTools_RegisterListDisableUnregisterRoundTrip(t *testing.T) {
	cs := connect(t)

	reg := call(t, cs, "register_mcp", map[string]any{
		"name":    "postgres-ro",
		"command": "mcp-server-postgres",
	})
	require.False(t, reg.IsError, "%s", reg.Content)
	var registered struct {
		Name      string `json:"name"`
		Transport string `json:"transport"`
		Status    string `json:"status"`
		Enabled   bool   `json:"enabled"`
		Tools     []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	decode(t, reg, &registered)
	assert.Equal(t, "postgres-ro", registered.Name)
	assert.Equal(t, "stdio", registered.Transport, "transport defaults to stdio")
	assert.Equal(t, cache.StatusActive, registered.Status)
	assert.True(t, registered.Enabled)
	require.Len(t, registered.Tools, 1)
	assert.Equal(t, "get_user", registered.Tools[0].Name)

	list := call(t, cs, "list_mcps", struct{}{})
	var listed struct {
		MCPs []struct {
			Name    string `json:"name"`
			Enabled bool   `json:"enabled"`
		} `json:"mcps"`
	}
	decode(t, list, &listed)
	require.Len(t, listed.MCPs, 1)
	assert.Equal(t, "postgres-ro", listed.MCPs[0].Name)

	dis := call(t, cs, "set_mcp_enabled", map[string]any{"name": "postgres-ro", "enabled": false})
	require.False(t, dis.IsError, "%s", dis.Content)
	var toggled struct {
		Enabled bool `json:"enabled"`
	}
	decode(t, dis, &toggled)
	assert.False(t, toggled.Enabled)

	del := call(t, cs, "unregister_mcp", map[string]any{"name": "postgres-ro"})
	require.False(t, del.IsError, "%s", del.Content)

	after := call(t, cs, "list_mcps", struct{}{})
	var empty struct {
		MCPs []json.RawMessage `json:"mcps"`
	}
	decode(t, after, &empty)
	assert.Empty(t, empty.MCPs)
}

func TestMCPTools_RegisterMCP_ThreadsEnvThroughToTheView(t *testing.T) {
	cs := connect(t)

	reg := call(t, cs, "register_mcp", map[string]any{
		"name":    "postgres-ro",
		"command": "mcp-server-postgres",
		"env":     map[string]string{"PGCONNECT_TIMEOUT": "5"},
	})
	require.False(t, reg.IsError, "%s", reg.Content)

	var registered struct {
		Env map[string]string `json:"env"`
	}
	decode(t, reg, &registered)
	assert.Equal(t, map[string]string{"PGCONNECT_TIMEOUT": "5"}, registered.Env)
}

func TestMCPTools_SetEnabledUnknownNameIsToolError(t *testing.T) {
	cs := connect(t)

	res := call(t, cs, "set_mcp_enabled", map[string]any{"name": "nope", "enabled": false})

	assert.True(t, res.IsError)
}

func TestAccessPolicyTools_CreateGetDeleteRoundTrip(t *testing.T) {
	cs := connect(t)

	created := call(t, cs, "create_access_policy", map[string]any{
		"name":   "db-reader",
		"match":  []map[string]any{{"path": "$.role", "pattern": "^db-reader$"}},
		"grants": []map[string]any{{"mcp": "postgres-ro", "tools": []string{"*"}}},
	})
	require.False(t, created.IsError, "%s", created.Content)

	got := call(t, cs, "get_access_policy", map[string]any{"name": "db-reader"})
	require.False(t, got.IsError, "%s", got.Content)
	var policy struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
		Match   []struct {
			Path string `json:"path"`
		} `json:"match"`
	}
	decode(t, got, &policy)
	assert.Equal(t, "db-reader", policy.Name)
	assert.True(t, policy.Enabled)
	require.Len(t, policy.Match, 1)
	assert.Equal(t, "$.role", policy.Match[0].Path)

	del := call(t, cs, "delete_access_policy", map[string]any{"name": "db-reader"})
	require.False(t, del.IsError, "%s", del.Content)

	missing := call(t, cs, "get_access_policy", map[string]any{"name": "db-reader"})
	assert.True(t, missing.IsError)
}

func TestAccessPolicyTools_MalformedRuleIsToolError(t *testing.T) {
	cs := connect(t)

	res := call(t, cs, "create_access_policy", map[string]any{
		"name":  "broken",
		"match": []map[string]any{{"path": "not valid $$", "pattern": "^x$"}},
	})

	assert.True(t, res.IsError)
}

func TestFilterPolicyTools_CreateAndList(t *testing.T) {
	cs := connect(t)

	created := call(t, cs, "create_filter_policy", map[string]any{
		"name":        "hide-pii",
		"mcp":         "postgres-ro",
		"tool":        "get_user",
		"drop_fields": []string{"$.hashed_password"},
	})
	require.False(t, created.IsError, "%s", created.Content)

	list := call(t, cs, "list_filter_policies", struct{}{})
	var listed struct {
		Policies []struct {
			Name       string   `json:"name"`
			MCP        string   `json:"mcp"`
			DropFields []string `json:"drop_fields"`
		} `json:"policies"`
	}
	decode(t, list, &listed)
	require.Len(t, listed.Policies, 1)
	assert.Equal(t, "hide-pii", listed.Policies[0].Name)
	assert.Equal(t, []string{"$.hashed_password"}, listed.Policies[0].DropFields)
}

func TestGatewayHealthTool(t *testing.T) {
	cs := connect(t)

	res := call(t, cs, "gateway_health", struct{}{})
	require.False(t, res.IsError)

	var out struct {
		Status string `json:"status"`
	}
	decode(t, res, &out)
	assert.Equal(t, "ok", out.Status)
}
