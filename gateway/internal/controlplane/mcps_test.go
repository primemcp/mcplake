package controlplane_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/atsokha/mcplake/cache"
	"github.com/atsokha/mcplake/internal/controlplane"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeMCPRegistry is a minimal in-memory stand-in for *cache.Registry, so
// handler tests can control success/failure without a real MCP subprocess.
// cache.Registry's own registration semantics (make-before-break,
// unreachable tracking, etc.) are already covered by the cache package's own
// tests; these tests exercise the handler's orchestration on top of it.
type fakeMCPRegistry struct {
	mu            sync.Mutex
	regs          map[string]cache.MCPRegistration
	registerErr   error
	unregisterErr error
}

func newFakeMCPRegistry() *fakeMCPRegistry {
	return &fakeMCPRegistry{regs: make(map[string]cache.MCPRegistration)}
}

func (f *fakeMCPRegistry) Register(_ context.Context, reg cache.MCPRegistration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.registerErr != nil {
		return f.registerErr
	}
	reg.Status = cache.StatusActive
	reg.Tools = map[string]cache.ToolSchema{"get_user": {Name: "get_user"}}
	f.regs[reg.Name] = reg
	return nil
}

func (f *fakeMCPRegistry) Unregister(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.unregisterErr != nil {
		return f.unregisterErr
	}
	if _, ok := f.regs[name]; !ok {
		return fmt.Errorf("cache: %q is not registered", name)
	}
	delete(f.regs, name)
	return nil
}

func (f *fakeMCPRegistry) Get(name string) (cache.MCPRegistration, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	reg, ok := f.regs[name]
	return reg, ok
}

func (f *fakeMCPRegistry) List() []cache.MCPRegistration {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]cache.MCPRegistration, 0, len(f.regs))
	for _, reg := range f.regs {
		out = append(out, reg)
	}
	return out
}

// fakeMCPRepository is a minimal in-memory stand-in for
// *persistence.MCPRegistrationRepo.
type fakeMCPRepository struct {
	mu        sync.Mutex
	rows      map[string]cache.MCPRegistration
	upsertErr error
	deleteErr error
}

func newFakeMCPRepository() *fakeMCPRepository {
	return &fakeMCPRepository{rows: make(map[string]cache.MCPRegistration)}
}

func (f *fakeMCPRepository) Upsert(_ context.Context, reg cache.MCPRegistration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.upsertErr != nil {
		return f.upsertErr
	}
	f.rows[reg.Name] = reg
	return nil
}

func (f *fakeMCPRepository) Delete(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.deleteErr != nil {
		return f.deleteErr
	}
	delete(f.rows, name)
	return nil
}

func newMCPTestRouter(registry *fakeMCPRegistry, repo *fakeMCPRepository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	admin := engine.Group("/admin")
	controlplane.RegisterMCPRoutes(admin, registry, repo)
	return engine
}

func doJSON(t *testing.T, engine *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		data, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(data)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

func TestRegisterMCPRoutes_PostRegistersAndPersists(t *testing.T) {
	registry := newFakeMCPRegistry()
	repo := newFakeMCPRepository()
	engine := newMCPTestRouter(registry, repo)

	rec := doJSON(t, engine, http.MethodPost, "/admin/mcps", map[string]any{
		"name":      "postgres-ro",
		"transport": "stdio",
		"connect":   map[string]any{"command": "mcp-server-postgres", "arguments": []string{"--read-only"}},
	})

	require.Equal(t, http.StatusCreated, rec.Code)

	var got map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "postgres-ro", got["name"])
	assert.Equal(t, cache.StatusActive, got["status"])

	// Immediately visible to the registry (what the data plane consults).
	reg, ok := registry.Get("postgres-ro")
	require.True(t, ok)
	assert.Equal(t, cache.StatusActive, reg.Status)

	// Persisted.
	repo.mu.Lock()
	row, ok := repo.rows["postgres-ro"]
	repo.mu.Unlock()
	require.True(t, ok)
	assert.Equal(t, cache.StatusActive, row.Status)
}

func TestRegisterMCPRoutes_PostMissingNameReturns400(t *testing.T) {
	engine := newMCPTestRouter(newFakeMCPRegistry(), newFakeMCPRepository())

	rec := doJSON(t, engine, http.MethodPost, "/admin/mcps", map[string]any{
		"transport": "stdio",
	})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRegisterMCPRoutes_PostMalformedJSONReturns400(t *testing.T) {
	engine := newMCPTestRouter(newFakeMCPRegistry(), newFakeMCPRepository())

	req := httptest.NewRequest(http.MethodPost, "/admin/mcps", bytes.NewBufferString("{not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRegisterMCPRoutes_PostRegistrationFailureIsNotPersistedOrVisible(t *testing.T) {
	registry := newFakeMCPRegistry()
	registry.registerErr = fmt.Errorf("cache: connect to %q: connection refused", "broken")
	repo := newFakeMCPRepository()
	engine := newMCPTestRouter(registry, repo)

	rec := doJSON(t, engine, http.MethodPost, "/admin/mcps", map[string]any{
		"name": "broken", "connect": map[string]any{"command": "does-not-exist"},
	})

	assert.Equal(t, http.StatusBadGateway, rec.Code)
	_, ok := registry.Get("broken")
	assert.False(t, ok)
	repo.mu.Lock()
	_, persisted := repo.rows["broken"]
	repo.mu.Unlock()
	assert.False(t, persisted)
}

func TestRegisterMCPRoutes_GetListsRegisteredMCPs(t *testing.T) {
	registry := newFakeMCPRegistry()
	repo := newFakeMCPRepository()
	engine := newMCPTestRouter(registry, repo)
	require.Equal(t, http.StatusCreated, doJSON(t, engine, http.MethodPost, "/admin/mcps", map[string]any{
		"name": "postgres-ro", "connect": map[string]any{"command": "mcp-server-postgres"},
	}).Code)

	rec := doJSON(t, engine, http.MethodGet, "/admin/mcps", nil)

	require.Equal(t, http.StatusOK, rec.Code)
	var got []map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got, 1)
	assert.Equal(t, "postgres-ro", got[0]["name"])
}

func TestRegisterMCPRoutes_DeleteUnregistersAndDeletes(t *testing.T) {
	registry := newFakeMCPRegistry()
	repo := newFakeMCPRepository()
	engine := newMCPTestRouter(registry, repo)
	require.Equal(t, http.StatusCreated, doJSON(t, engine, http.MethodPost, "/admin/mcps", map[string]any{
		"name": "postgres-ro", "connect": map[string]any{"command": "mcp-server-postgres"},
	}).Code)

	rec := doJSON(t, engine, http.MethodDelete, "/admin/mcps/postgres-ro", nil)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	_, ok := registry.Get("postgres-ro")
	assert.False(t, ok)
	repo.mu.Lock()
	_, persisted := repo.rows["postgres-ro"]
	repo.mu.Unlock()
	assert.False(t, persisted)
}

func TestRegisterMCPRoutes_DeleteUnknownNameReturns404(t *testing.T) {
	engine := newMCPTestRouter(newFakeMCPRegistry(), newFakeMCPRepository())

	rec := doJSON(t, engine, http.MethodDelete, "/admin/mcps/does-not-exist", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// Compile-time check that *cache.Registry satisfies the mcpRegistry
// interface RegisterMCPRoutes needs (the real implementation used in
// production, per cmd/gateway's future wiring, ticket #51).
var _ = func() {
	var _ interface {
		Register(ctx context.Context, reg cache.MCPRegistration) error
		Unregister(name string) error
		Get(name string) (cache.MCPRegistration, bool)
		List() []cache.MCPRegistration
	} = (*cache.Registry)(nil)
}
