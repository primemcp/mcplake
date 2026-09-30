// Package persistence implements the GORM-backed durable store for
// MCPRegistration, AccessPolicy, and FilterPolicy records. See
// docs/architecture/components.rst#persistence-layer-persistence,
// docs/architecture/data.rst#persistence-models-gorm, and
// docs/architecture/decisions/0006-gorm-sqlite-postgres-persistence.rst.
package persistence

import (
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// enabledColumn is the shared name of the operator on/off column across all
// three rows, listed in each repo's OnConflict DoUpdates so a toggle
// persists on update, not just on insert.
const enabledColumn = "enabled"

// enabledValue resolves a nullable `enabled` column to a concrete bool. A
// NULL (nil) — a legacy row written before the column existed — reads as
// enabled, matching the column's DB-level default.
func enabledValue(enabled *bool) bool {
	return enabled == nil || *enabled
}

// MCPRegistrationRow is the durable form of a cache.MCPRegistration. Connect
// and Tools are stored as JSON text columns (dialect-portable across SQLite
// and PostgreSQL, per ADR-0006) rather than dialect-specific types; the live
// cache.MCPClient is never persisted, only the data needed to reconstruct
// registration and re-discover it.
type MCPRegistrationRow struct {
	gorm.Model
	Name      string `gorm:"uniqueIndex"`
	Transport string
	Connect   datatypes.JSON
	Tools     datatypes.JSON
	Status    string
	// Enabled is the operator on/off switch (see #95). It is a pointer with
	// a DB-level default of true so AutoMigrate backfills existing rows as
	// enabled when the column is first added, and so an explicit false is
	// never mistaken by GORM for "unset" and overwritten by the default on
	// write.
	Enabled *bool `gorm:"not null;default:true"`
}

// AccessPolicyRow is the durable form of a router.AccessPolicy. Match holds
// the []ClaimRule{Path,Pattern} pairs (not the compiled matcher), Grants the
// []router.Grant list, both as JSON.
type AccessPolicyRow struct {
	gorm.Model
	Name    string `gorm:"uniqueIndex"`
	Match   datatypes.JSON
	Grants  datatypes.JSON
	Enabled *bool `gorm:"not null;default:true"`
}

// FilterPolicyRow is the durable form of a router.FilterPolicy.
type FilterPolicyRow struct {
	gorm.Model
	Name       string `gorm:"uniqueIndex"`
	Match      datatypes.JSON
	MCP        string
	Tool       string
	DropFields datatypes.JSON
	Enabled    *bool `gorm:"not null;default:true"`
}
