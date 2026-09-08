package controlplane_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Access policies -------------------------------------------------------

func TestAccessPolicyRoutes_CreateDefaultsEnabledToTrue(t *testing.T) {
	engine, repo, store := newAccessPolicyTestRouter(t)

	rec := doJSON(t, engine, http.MethodPost, "/admin/access-policies", map[string]any{
		"name":   "db-reader",
		"match":  []map[string]any{{"path": "$.role", "pattern": "^db-reader$"}},
		"grants": []map[string]any{{"mcp": "postgres-ro", "tools": []string{"*"}}},
	})
	require.Equal(t, http.StatusCreated, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, true, body["enabled"], "response echoes enabled")

	stored, ok, _ := repo.Get(context.Background(), "db-reader")
	require.True(t, ok)
	assert.True(t, stored.Enabled)

	authorized, err := store.Authorize([]byte(`{"role":"db-reader"}`), "postgres-ro", "get_user")
	require.NoError(t, err)
	assert.True(t, authorized)
}

func TestAccessPolicyRoutes_CreateWithEnabledFalsePersistsAndGrantsNothing(t *testing.T) {
	engine, repo, store := newAccessPolicyTestRouter(t)

	rec := doJSON(t, engine, http.MethodPost, "/admin/access-policies", map[string]any{
		"name":    "db-reader",
		"enabled": false,
		"match":   []map[string]any{{"path": "$.role", "pattern": "^db-reader$"}},
		"grants":  []map[string]any{{"mcp": "postgres-ro", "tools": []string{"*"}}},
	})
	require.Equal(t, http.StatusCreated, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, false, body["enabled"])

	stored, ok, _ := repo.Get(context.Background(), "db-reader")
	require.True(t, ok)
	assert.False(t, stored.Enabled)

	authorized, err := store.Authorize([]byte(`{"role":"db-reader"}`), "postgres-ro", "get_user")
	require.NoError(t, err)
	assert.False(t, authorized, "a disabled policy created via the API grants nothing")
}

func TestAccessPolicyRoutes_PutWithoutEnabledReEnables(t *testing.T) {
	engine, repo, _ := newAccessPolicyTestRouter(t)

	require.Equal(t, http.StatusCreated, doJSON(t, engine, http.MethodPost, "/admin/access-policies", map[string]any{
		"name":    "db-reader",
		"enabled": false,
		"match":   []map[string]any{{"path": "$.role", "pattern": "^db-reader$"}},
		"grants":  []map[string]any{{"mcp": "postgres-ro", "tools": []string{"*"}}},
	}).Code)

	// PUT replaces the whole record; omitting enabled resets it to the default.
	rec := doJSON(t, engine, http.MethodPut, "/admin/access-policies/db-reader", map[string]any{
		"match":  []map[string]any{{"path": "$.role", "pattern": "^db-reader$"}},
		"grants": []map[string]any{{"mcp": "postgres-ro", "tools": []string{"*"}}},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	stored, ok, _ := repo.Get(context.Background(), "db-reader")
	require.True(t, ok)
	assert.True(t, stored.Enabled, "PUT without enabled re-enables the policy")
}

// --- Filter policies -----------------------------------------------------

func TestFilterPolicyRoutes_CreateWithEnabledFalsePersistsAndStripsNothing(t *testing.T) {
	engine, repo, store := newFilterPolicyTestRouter(t)

	rec := doJSON(t, engine, http.MethodPost, "/admin/filter-policies", map[string]any{
		"name":        "hide-pii",
		"enabled":     false,
		"match":       []map[string]any{{"path": "$.role", "pattern": "^user$"}},
		"mcp":         "postgres-ro",
		"tool":        "get_user",
		"drop_fields": []string{"$.ssn"},
	})
	require.Equal(t, http.StatusCreated, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, false, body["enabled"])

	stored, ok, _ := repo.Get(context.Background(), "hide-pii")
	require.True(t, ok)
	assert.False(t, stored.Enabled)

	fields, err := store.FieldsToRemove([]byte(`{"role":"user"}`), "postgres-ro", "get_user")
	require.NoError(t, err)
	assert.Empty(t, fields, "a disabled filter policy created via the API strips nothing")
}

func TestFilterPolicyRoutes_CreateDefaultsEnabledToTrue(t *testing.T) {
	engine, repo, _ := newFilterPolicyTestRouter(t)

	rec := doJSON(t, engine, http.MethodPost, "/admin/filter-policies", map[string]any{
		"name":        "hide-pii",
		"match":       []map[string]any{{"path": "$.role", "pattern": "^user$"}},
		"mcp":         "postgres-ro",
		"tool":        "get_user",
		"drop_fields": []string{"$.ssn"},
	})
	require.Equal(t, http.StatusCreated, rec.Code)

	stored, ok, _ := repo.Get(context.Background(), "hide-pii")
	require.True(t, ok)
	assert.True(t, stored.Enabled)
}

// --- MCPs: enabled on register + PATCH toggle ---------------------------

func TestRegisterMCPRoutes_PostDefaultsEnabledToTrue(t *testing.T) {
	registry := newFakeMCPRegistry()
	repo := newFakeMCPRepository()
	engine := newMCPTestRouter(registry, repo)

	rec := doJSON(t, engine, http.MethodPost, "/admin/mcps", map[string]any{
		"name":    "postgres-ro",
		"connect": map[string]any{"command": "mcp-server-postgres"},
	})
	require.Equal(t, http.StatusCreated, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, true, body["enabled"])

	reg, ok := registry.Get("postgres-ro")
	require.True(t, ok)
	assert.True(t, reg.Enabled)
}

func TestMCPRoutes_PatchTogglesEnabledInPlaceAndPersists(t *testing.T) {
	registry := newFakeMCPRegistry()
	repo := newFakeMCPRepository()
	engine := newMCPTestRouter(registry, repo)

	require.Equal(t, http.StatusCreated, doJSON(t, engine, http.MethodPost, "/admin/mcps", map[string]any{
		"name":    "postgres-ro",
		"connect": map[string]any{"command": "mcp-server-postgres"},
	}).Code)

	rec := doJSON(t, engine, http.MethodPatch, "/admin/mcps/postgres-ro", map[string]any{"enabled": false})
	require.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, false, body["enabled"])
	assert.Equal(t, "active", body["status"], "the registration is unchanged apart from enabled — no reconnect")

	reg, ok := registry.Get("postgres-ro")
	require.True(t, ok)
	assert.False(t, reg.Enabled, "the live registry reflects the toggle immediately")
	assert.NotEmpty(t, reg.Tools, "tools stay cached across a disable")

	row, ok := repo.rows["postgres-ro"]
	require.True(t, ok)
	assert.False(t, row.Enabled, "the toggle is persisted")

	// Re-enable.
	rec = doJSON(t, engine, http.MethodPatch, "/admin/mcps/postgres-ro", map[string]any{"enabled": true})
	require.Equal(t, http.StatusOK, rec.Code)
	reg, _ = registry.Get("postgres-ro")
	assert.True(t, reg.Enabled)
}

func TestMCPRoutes_PatchUnknownNameReturns404(t *testing.T) {
	engine := newMCPTestRouter(newFakeMCPRegistry(), newFakeMCPRepository())

	rec := doJSON(t, engine, http.MethodPatch, "/admin/mcps/nope", map[string]any{"enabled": false})

	assert.Equal(t, http.StatusNotFound, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "not_found", body["error"])
}

func TestMCPRoutes_PatchWithoutEnabledFieldReturns400(t *testing.T) {
	registry := newFakeMCPRegistry()
	engine := newMCPTestRouter(registry, newFakeMCPRepository())
	require.Equal(t, http.StatusCreated, doJSON(t, engine, http.MethodPost, "/admin/mcps", map[string]any{
		"name":    "postgres-ro",
		"connect": map[string]any{"command": "mcp-server-postgres"},
	}).Code)

	rec := doJSON(t, engine, http.MethodPatch, "/admin/mcps/postgres-ro", map[string]any{})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "invalid_request", body["error"])
}
