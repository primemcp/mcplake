package persistence_test

import (
	"path/filepath"
	"testing"

	"github.com/atsokha/mcplake/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := persistence.Open(persistence.Config{DSN: path})
	require.NoError(t, err)
	return db
}

func TestOpen_SQLiteDefaultMigratesAllTables(t *testing.T) {
	db := openTestDB(t)

	assert.True(t, db.Migrator().HasTable(&persistence.MCPRegistrationRow{}))
	assert.True(t, db.Migrator().HasTable(&persistence.AccessPolicyRow{}))
	assert.True(t, db.Migrator().HasTable(&persistence.FilterPolicyRow{}))
}

func TestOpen_UnsupportedDriverReturnsError(t *testing.T) {
	_, err := persistence.Open(persistence.Config{Driver: "mysql"})
	require.Error(t, err)
}

func TestOpen_PostgresDriverSelectsPostgresDialector(t *testing.T) {
	// A live Postgres server is not required here (and not available in this
	// environment) — the assertion is that Open reaches the connection
	// attempt via the Postgres dialector rather than being rejected as an
	// unsupported driver.
	_, err := persistence.Open(persistence.Config{Driver: persistence.DriverPostgres, DSN: "host=127.0.0.1 port=1 sslmode=disable"})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "unsupported driver")
}

func TestOpen_MCPRegistrationRowRoundTripsJSONColumns(t *testing.T) {
	db := openTestDB(t)

	row := persistence.MCPRegistrationRow{
		Name:      "postgres-ro",
		Transport: "stdio",
		Connect:   datatypes.JSON(`{"command":"mcp-server-postgres","arguments":["--read-only"]}`),
		Tools:     datatypes.JSON(`{"get_user":{"name":"get_user"}}`),
		Status:    "active",
	}
	require.NoError(t, db.Create(&row).Error)

	var got persistence.MCPRegistrationRow
	require.NoError(t, db.First(&got, "name = ?", "postgres-ro").Error)

	assert.Equal(t, "stdio", got.Transport)
	assert.JSONEq(t, `{"command":"mcp-server-postgres","arguments":["--read-only"]}`, string(got.Connect))
	assert.JSONEq(t, `{"get_user":{"name":"get_user"}}`, string(got.Tools))
	assert.Equal(t, "active", got.Status)
}

func TestOpen_AccessPolicyRowRoundTripsJSONColumns(t *testing.T) {
	db := openTestDB(t)

	row := persistence.AccessPolicyRow{
		Name:   "db-reader",
		Match:  datatypes.JSON(`[{"path":"$.role","pattern":"^db-reader$"}]`),
		Grants: datatypes.JSON(`[{"mcp":"postgres-ro","tools":["*"]}]`),
	}
	require.NoError(t, db.Create(&row).Error)

	var got persistence.AccessPolicyRow
	require.NoError(t, db.First(&got, "name = ?", "db-reader").Error)

	assert.JSONEq(t, `[{"path":"$.role","pattern":"^db-reader$"}]`, string(got.Match))
	assert.JSONEq(t, `[{"mcp":"postgres-ro","tools":["*"]}]`, string(got.Grants))
}

func TestOpen_FilterPolicyRowRoundTripsJSONColumns(t *testing.T) {
	db := openTestDB(t)

	row := persistence.FilterPolicyRow{
		Name:       "hide-pii",
		Match:      datatypes.JSON(`[{"path":"$.role","pattern":"^user$"}]`),
		MCP:        "postgres-ro",
		Tool:       "get_user",
		DropFields: datatypes.JSON(`["$.hashed_password"]`),
	}
	require.NoError(t, db.Create(&row).Error)

	var got persistence.FilterPolicyRow
	require.NoError(t, db.First(&got, "name = ?", "hide-pii").Error)

	assert.Equal(t, "postgres-ro", got.MCP)
	assert.Equal(t, "get_user", got.Tool)
	assert.JSONEq(t, `["$.hashed_password"]`, string(got.DropFields))
}

func TestOpen_UniqueNameConstraintRejectsDuplicateInsert(t *testing.T) {
	db := openTestDB(t)

	require.NoError(t, db.Create(&persistence.AccessPolicyRow{Name: "dup"}).Error)
	err := db.Create(&persistence.AccessPolicyRow{Name: "dup"}).Error

	require.Error(t, err)
}
