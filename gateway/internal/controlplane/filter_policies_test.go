package controlplane_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/atsokha/mcplake/internal/controlplane"
	"github.com/atsokha/mcplake/router"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeFilterPolicyRepoStore is a minimal in-memory stand-in for
// *persistence.FilterPolicyRepo, validating/compiling Match rules the same
// way the real repository does.
type fakeFilterPolicyRepoStore struct {
	mu       sync.Mutex
	policies map[string]router.FilterPolicy
}

func newFakeFilterPolicyRepoStore() *fakeFilterPolicyRepoStore {
	return &fakeFilterPolicyRepoStore{policies: make(map[string]router.FilterPolicy)}
}

func (f *fakeFilterPolicyRepoStore) Upsert(_ context.Context, policy router.FilterPolicy) error {
	compiled := make([]router.ClaimRule, len(policy.Match.Rules))
	for i, rule := range policy.Match.Rules {
		c, err := router.NewClaimRule(rule.Path, rule.Pattern)
		if err != nil {
			return err
		}
		compiled[i] = c
	}
	policy.Match = router.ClaimMatcher{Rules: compiled}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.policies[policy.Name] = policy
	return nil
}

func (f *fakeFilterPolicyRepoStore) Get(_ context.Context, name string) (router.FilterPolicy, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.policies[name]
	return p, ok, nil
}

func (f *fakeFilterPolicyRepoStore) List(_ context.Context) ([]router.FilterPolicy, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]router.FilterPolicy, 0, len(f.policies))
	for _, p := range f.policies {
		out = append(out, p)
	}
	return out, nil
}

func (f *fakeFilterPolicyRepoStore) Delete(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.policies, name)
	return nil
}

func newFilterPolicyTestRouter(t *testing.T) (*gin.Engine, *fakeFilterPolicyRepoStore, *router.PolicyStore) {
	t.Helper()
	accessRepo := newFakeAccessPolicyRepo()
	filterRepo := newFakeFilterPolicyRepoStore()
	store := router.NewPolicyStore(router.NewEngine(nil, nil))
	reloader := controlplane.NewPolicyReloader(accessRepo, filterRepo, store)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	admin := engine.Group("/admin")
	controlplane.RegisterFilterPolicyRoutes(admin, filterRepo, reloader)
	return engine, filterRepo, store
}

func TestFilterPolicyRoutes_CreateIsImmediatelyVisibleToPolicyStore(t *testing.T) {
	engine, _, store := newFilterPolicyTestRouter(t)

	rec := doJSON(t, engine, http.MethodPost, "/admin/filter-policies", map[string]any{
		"name":        "hide-pii",
		"match":       []map[string]any{{"path": "$.role", "pattern": "^user$"}},
		"mcp":         "postgres-ro",
		"tool":        "get_user",
		"drop_fields": []string{"$.hashed_password"},
	})

	require.Equal(t, http.StatusCreated, rec.Code)

	fields, err := store.FieldsToRemove([]byte(`{"role":"user"}`), "postgres-ro", "get_user")
	require.NoError(t, err)
	assert.Equal(t, []string{"$.hashed_password"}, fields)
}

func TestFilterPolicyRoutes_CreateMissingNameReturns400(t *testing.T) {
	engine, _, _ := newFilterPolicyTestRouter(t)

	rec := doJSON(t, engine, http.MethodPost, "/admin/filter-policies", map[string]any{
		"mcp": "postgres-ro", "tool": "get_user",
	})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestFilterPolicyRoutes_CreateMissingMCPOrToolReturns400(t *testing.T) {
	engine, _, _ := newFilterPolicyTestRouter(t)

	rec := doJSON(t, engine, http.MethodPost, "/admin/filter-policies", map[string]any{
		"name": "hide-pii",
	})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestFilterPolicyRoutes_CreateMalformedRuleReturns400AndDoesNotPersist(t *testing.T) {
	engine, repo, _ := newFilterPolicyTestRouter(t)

	rec := doJSON(t, engine, http.MethodPost, "/admin/filter-policies", map[string]any{
		"name": "broken", "mcp": "postgres-ro", "tool": "get_user",
		"match": []map[string]any{{"path": "$.role", "pattern": "(unclosed"}},
	})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	_, ok, _ := repo.Get(context.Background(), "broken")
	assert.False(t, ok)
}

func TestFilterPolicyRoutes_PutReplacesDropFields(t *testing.T) {
	engine, _, store := newFilterPolicyTestRouter(t)
	require.Equal(t, http.StatusCreated, doJSON(t, engine, http.MethodPost, "/admin/filter-policies", map[string]any{
		"name": "hide-pii", "mcp": "postgres-ro", "tool": "get_user",
		"match":       []map[string]any{{"path": "$.role", "pattern": "^user$"}},
		"drop_fields": []string{"$.hashed_password"},
	}).Code)

	rec := doJSON(t, engine, http.MethodPut, "/admin/filter-policies/hide-pii", map[string]any{
		"mcp": "postgres-ro", "tool": "get_user",
		"match":       []map[string]any{{"path": "$.role", "pattern": "^user$"}},
		"drop_fields": []string{"$.email"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	fields, err := store.FieldsToRemove([]byte(`{"role":"user"}`), "postgres-ro", "get_user")
	require.NoError(t, err)
	assert.Equal(t, []string{"$.email"}, fields)
}

func TestFilterPolicyRoutes_GetReturnsPolicy(t *testing.T) {
	engine, _, _ := newFilterPolicyTestRouter(t)
	require.Equal(t, http.StatusCreated, doJSON(t, engine, http.MethodPost, "/admin/filter-policies", map[string]any{
		"name": "hide-pii", "mcp": "postgres-ro", "tool": "get_user",
	}).Code)

	rec := doJSON(t, engine, http.MethodGet, "/admin/filter-policies/hide-pii", nil)

	require.Equal(t, http.StatusOK, rec.Code)
	var got map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "hide-pii", got["name"])
}

func TestFilterPolicyRoutes_GetUnknownNameReturns404(t *testing.T) {
	engine, _, _ := newFilterPolicyTestRouter(t)

	rec := doJSON(t, engine, http.MethodGet, "/admin/filter-policies/does-not-exist", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestFilterPolicyRoutes_ListReturnsAll(t *testing.T) {
	engine, _, _ := newFilterPolicyTestRouter(t)
	require.Equal(t, http.StatusCreated, doJSON(t, engine, http.MethodPost, "/admin/filter-policies", map[string]any{
		"name": "a", "mcp": "postgres-ro", "tool": "get_user",
	}).Code)
	require.Equal(t, http.StatusCreated, doJSON(t, engine, http.MethodPost, "/admin/filter-policies", map[string]any{
		"name": "b", "mcp": "postgres-ro", "tool": "list_users",
	}).Code)

	rec := doJSON(t, engine, http.MethodGet, "/admin/filter-policies", nil)

	require.Equal(t, http.StatusOK, rec.Code)
	var got []map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Len(t, got, 2)
}

func TestFilterPolicyRoutes_DeleteRemovesPolicyAndFieldsStopBeingStripped(t *testing.T) {
	engine, _, store := newFilterPolicyTestRouter(t)
	require.Equal(t, http.StatusCreated, doJSON(t, engine, http.MethodPost, "/admin/filter-policies", map[string]any{
		"name": "hide-pii", "mcp": "postgres-ro", "tool": "get_user",
		"match":       []map[string]any{{"path": "$.role", "pattern": "^user$"}},
		"drop_fields": []string{"$.hashed_password"},
	}).Code)

	rec := doJSON(t, engine, http.MethodDelete, "/admin/filter-policies/hide-pii", nil)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	fields, err := store.FieldsToRemove([]byte(`{"role":"user"}`), "postgres-ro", "get_user")
	require.NoError(t, err)
	assert.Empty(t, fields)
}

func TestFilterPolicyRoutes_DeleteUnknownNameReturns404(t *testing.T) {
	engine, _, _ := newFilterPolicyTestRouter(t)

	rec := doJSON(t, engine, http.MethodDelete, "/admin/filter-policies/does-not-exist", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestFilterPolicyRoutes_MalformedJSONReturns400(t *testing.T) {
	engine, _, _ := newFilterPolicyTestRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/admin/filter-policies", bytes.NewBufferString("{not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
