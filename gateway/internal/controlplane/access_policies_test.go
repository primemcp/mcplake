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
	"github.com/atsokha/mcplake/internal/controlplane/adminservice"
	"github.com/atsokha/mcplake/router"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeAccessPolicyRepo is a minimal in-memory stand-in for
// *persistence.AccessPolicyRepo, validating/compiling Match rules the same
// way the real repository does (reject a malformed rule, never store it).
type fakeAccessPolicyRepo struct {
	mu       sync.Mutex
	policies map[string]router.AccessPolicy
}

func newFakeAccessPolicyRepo() *fakeAccessPolicyRepo {
	return &fakeAccessPolicyRepo{policies: make(map[string]router.AccessPolicy)}
}

func (f *fakeAccessPolicyRepo) Upsert(_ context.Context, policy router.AccessPolicy) error {
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

func (f *fakeAccessPolicyRepo) Get(_ context.Context, name string) (router.AccessPolicy, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.policies[name]
	return p, ok, nil
}

func (f *fakeAccessPolicyRepo) List(_ context.Context) ([]router.AccessPolicy, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]router.AccessPolicy, 0, len(f.policies))
	for _, p := range f.policies {
		out = append(out, p)
	}
	return out, nil
}

func (f *fakeAccessPolicyRepo) Delete(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.policies, name)
	return nil
}

// fakeFilterPolicyRepo is an always-empty stand-in, used where these tests
// only care about access policies.
type fakeFilterPolicyRepo struct{}

func (fakeFilterPolicyRepo) Upsert(context.Context, router.FilterPolicy) error { return nil }
func (fakeFilterPolicyRepo) Get(context.Context, string) (router.FilterPolicy, bool, error) {
	return router.FilterPolicy{}, false, nil
}
func (fakeFilterPolicyRepo) List(context.Context) ([]router.FilterPolicy, error) { return nil, nil }
func (fakeFilterPolicyRepo) Delete(context.Context, string) error                { return nil }

func newAccessPolicyTestRouter(t *testing.T) (*gin.Engine, *fakeAccessPolicyRepo, *router.PolicyStore) {
	t.Helper()
	repo := newFakeAccessPolicyRepo()
	store := router.NewPolicyStore(router.NewEngine(nil, nil))
	reloader := controlplane.NewPolicyReloader(repo, fakeFilterPolicyRepo{}, store)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	admin := engine.Group("/admin")
	controlplane.RegisterAccessPolicyRoutes(admin, adminservice.NewAccessPolicyService(repo, reloader))
	return engine, repo, store
}

func TestAccessPolicyRoutes_CreateIsImmediatelyVisibleToPolicyStore(t *testing.T) {
	engine, _, store := newAccessPolicyTestRouter(t)

	rec := doJSON(t, engine, http.MethodPost, "/admin/access-policies", map[string]any{
		"name":   "db-reader",
		"match":  []map[string]any{{"path": "$.role", "pattern": "^db-reader$"}},
		"grants": []map[string]any{{"mcp": "postgres-ro", "tools": []string{"*"}}},
	})

	require.Equal(t, http.StatusCreated, rec.Code)

	authorized, err := store.Authorize([]byte(`{"role":"db-reader"}`), "postgres-ro", "get_user")
	require.NoError(t, err)
	assert.True(t, authorized)
}

func TestAccessPolicyRoutes_CreateMissingNameReturns400(t *testing.T) {
	engine, _, _ := newAccessPolicyTestRouter(t)

	rec := doJSON(t, engine, http.MethodPost, "/admin/access-policies", map[string]any{
		"match": []map[string]any{{"path": "$.role", "pattern": "^db-reader$"}},
	})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAccessPolicyRoutes_CreateMalformedRuleReturns400AndDoesNotPersist(t *testing.T) {
	engine, repo, _ := newAccessPolicyTestRouter(t)

	rec := doJSON(t, engine, http.MethodPost, "/admin/access-policies", map[string]any{
		"name":  "broken",
		"match": []map[string]any{{"path": "not a valid jsonpath $$", "pattern": "^x$"}},
	})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	_, ok, _ := repo.Get(context.Background(), "broken")
	assert.False(t, ok)
}

func TestAccessPolicyRoutes_PutReplacesGrantsAndOldNoLongerMatches(t *testing.T) {
	engine, _, store := newAccessPolicyTestRouter(t)
	require.Equal(t, http.StatusCreated, doJSON(t, engine, http.MethodPost, "/admin/access-policies", map[string]any{
		"name":   "db-reader",
		"match":  []map[string]any{{"path": "$.role", "pattern": "^db-reader$"}},
		"grants": []map[string]any{{"mcp": "postgres-ro", "tools": []string{"*"}}},
	}).Code)

	rec := doJSON(t, engine, http.MethodPut, "/admin/access-policies/db-reader", map[string]any{
		"match":  []map[string]any{{"path": "$.role", "pattern": "^db-reader$"}},
		"grants": []map[string]any{{"mcp": "postgres-rw", "tools": []string{"get_user"}}},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	claims := []byte(`{"role":"db-reader"}`)
	oldGrant, err := store.Authorize(claims, "postgres-ro", "get_user")
	require.NoError(t, err)
	assert.False(t, oldGrant)

	newGrant, err := store.Authorize(claims, "postgres-rw", "get_user")
	require.NoError(t, err)
	assert.True(t, newGrant)
}

func TestAccessPolicyRoutes_GetReturnsPolicy(t *testing.T) {
	engine, _, _ := newAccessPolicyTestRouter(t)
	require.Equal(t, http.StatusCreated, doJSON(t, engine, http.MethodPost, "/admin/access-policies", map[string]any{
		"name": "db-reader", "match": []map[string]any{{"path": "$.role", "pattern": "^db-reader$"}},
	}).Code)

	rec := doJSON(t, engine, http.MethodGet, "/admin/access-policies/db-reader", nil)

	require.Equal(t, http.StatusOK, rec.Code)
	var got map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "db-reader", got["name"])
}

func TestAccessPolicyRoutes_GetUnknownNameReturns404(t *testing.T) {
	engine, _, _ := newAccessPolicyTestRouter(t)

	rec := doJSON(t, engine, http.MethodGet, "/admin/access-policies/does-not-exist", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestAccessPolicyRoutes_ListReturnsAll(t *testing.T) {
	engine, _, _ := newAccessPolicyTestRouter(t)
	require.Equal(t, http.StatusCreated, doJSON(t, engine, http.MethodPost, "/admin/access-policies", map[string]any{"name": "a"}).Code)
	require.Equal(t, http.StatusCreated, doJSON(t, engine, http.MethodPost, "/admin/access-policies", map[string]any{"name": "b"}).Code)

	rec := doJSON(t, engine, http.MethodGet, "/admin/access-policies", nil)

	require.Equal(t, http.StatusOK, rec.Code)
	var got []map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Len(t, got, 2)
}

func TestAccessPolicyRoutes_DeleteRemovesPolicyAndItStopsMatching(t *testing.T) {
	engine, _, store := newAccessPolicyTestRouter(t)
	require.Equal(t, http.StatusCreated, doJSON(t, engine, http.MethodPost, "/admin/access-policies", map[string]any{
		"name":   "db-reader",
		"match":  []map[string]any{{"path": "$.role", "pattern": "^db-reader$"}},
		"grants": []map[string]any{{"mcp": "postgres-ro", "tools": []string{"*"}}},
	}).Code)

	rec := doJSON(t, engine, http.MethodDelete, "/admin/access-policies/db-reader", nil)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	authorized, err := store.Authorize([]byte(`{"role":"db-reader"}`), "postgres-ro", "get_user")
	require.NoError(t, err)
	assert.False(t, authorized)
}

func TestAccessPolicyRoutes_DeleteUnknownNameReturns404(t *testing.T) {
	engine, _, _ := newAccessPolicyTestRouter(t)

	rec := doJSON(t, engine, http.MethodDelete, "/admin/access-policies/does-not-exist", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestAccessPolicyRoutes_MalformedJSONReturns400(t *testing.T) {
	engine, _, _ := newAccessPolicyTestRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/admin/access-policies", bytes.NewBufferString("{not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
