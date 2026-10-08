package persistence_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"

	"github.com/primemcp/mcplake/cache"
	"github.com/primemcp/mcplake/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMCPRegistrationRepo(t *testing.T) *persistence.MCPRegistrationRepo {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := persistence.Open(persistence.Config{DSN: path})
	require.NoError(t, err)
	return persistence.NewMCPRegistrationRepo(db)
}

func sampleRegistration() cache.MCPRegistration {
	return cache.MCPRegistration{
		Name:      "postgres-ro",
		Transport: "stdio",
		Connect: cache.ConnectConfig{
			Command:   "mcp-server-postgres",
			Arguments: []string{"--read-only"},
		},
		Status: cache.StatusActive,
		Tools: map[string]cache.ToolSchema{
			"get_user": {
				Name:         "get_user",
				InputSchema:  json.RawMessage(`{"type":"object"}`),
				OutputSchema: json.RawMessage(`{"type":"object"}`),
			},
		},
	}
}

func TestMCPRegistrationRepo_UpsertThenGetRoundTrips(t *testing.T) {
	repo := newMCPRegistrationRepo(t)
	ctx := context.Background()
	reg := sampleRegistration()

	require.NoError(t, repo.Upsert(ctx, reg))

	got, ok, err := repo.Get(ctx, "postgres-ro")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, reg.Name, got.Name)
	assert.Equal(t, reg.Transport, got.Transport)
	assert.Equal(t, reg.Connect, got.Connect)
	assert.Equal(t, reg.Status, got.Status)
	assert.Equal(t, reg.Tools, got.Tools)
	assert.Nil(t, got.Client)
}

func TestMCPRegistrationRepo_UpsertThenGetRoundTripsEnv(t *testing.T) {
	repo := newMCPRegistrationRepo(t)
	ctx := context.Background()
	reg := sampleRegistration()
	reg.Connect.Env = map[string]string{"API_KEY": "secret"}

	require.NoError(t, repo.Upsert(ctx, reg))

	got, ok, err := repo.Get(ctx, reg.Name)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, reg.Connect.Env, got.Connect.Env)
}

func TestMCPRegistrationRepo_GetUnknownNameReturnsNotFoundNotError(t *testing.T) {
	repo := newMCPRegistrationRepo(t)

	_, ok, err := repo.Get(context.Background(), "does-not-exist")

	require.NoError(t, err)
	assert.False(t, ok)
}

func TestMCPRegistrationRepo_UpsertOnExistingNameUpdatesInPlace(t *testing.T) {
	repo := newMCPRegistrationRepo(t)
	ctx := context.Background()
	reg := sampleRegistration()
	require.NoError(t, repo.Upsert(ctx, reg))

	reg.Status = cache.StatusUnreachable
	reg.Tools = map[string]cache.ToolSchema{}
	require.NoError(t, repo.Upsert(ctx, reg))

	got, ok, err := repo.Get(ctx, "postgres-ro")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, cache.StatusUnreachable, got.Status)
	assert.Empty(t, got.Tools)

	all, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 1, "upsert must not create a duplicate row")
}

func TestMCPRegistrationRepo_ListReturnsAllRegistrations(t *testing.T) {
	repo := newMCPRegistrationRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.Upsert(ctx, sampleRegistration()))
	other := sampleRegistration()
	other.Name = "postgres-rw"
	require.NoError(t, repo.Upsert(ctx, other))

	all, err := repo.List(ctx)

	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestMCPRegistrationRepo_DeleteRemovesRow(t *testing.T) {
	repo := newMCPRegistrationRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, sampleRegistration()))

	require.NoError(t, repo.Delete(ctx, "postgres-ro"))

	_, ok, err := repo.Get(ctx, "postgres-ro")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestMCPRegistrationRepo_DeleteThenUpsertSameNameSucceeds(t *testing.T) {
	// Regression test for gorm.Model's default soft-delete: a naive Delete
	// would leave the old row occupying the unique "name" index, causing a
	// subsequent re-registration of the same name to fail. Delete must hard
	// delete.
	repo := newMCPRegistrationRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, sampleRegistration()))
	require.NoError(t, repo.Delete(ctx, "postgres-ro"))

	require.NoError(t, repo.Upsert(ctx, sampleRegistration()))

	got, ok, err := repo.Get(ctx, "postgres-ro")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, cache.StatusActive, got.Status)
}

func TestMCPRegistrationRepo_ConcurrentUpsertsToDifferentNamesDoNotCorrupt(t *testing.T) {
	repo := newMCPRegistrationRepo(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		reg := sampleRegistration()
		reg.Name = "mcp-" + string(rune('a'+i))
		wg.Add(1)
		go func(r cache.MCPRegistration) {
			defer wg.Done()
			assert.NoError(t, repo.Upsert(ctx, r))
		}(reg)
	}
	wg.Wait()

	all, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 10)
}
