// Package persistence implements the GORM-backed durable store for
// MCPRegistration, AccessPolicy, and FilterPolicy records. See
// docs/architecture/components.md#persistence-layer-persistence,
// docs/architecture/data.md#persistence-models-gorm, and
// docs/architecture/decisions/0006-gorm-sqlite-postgres-persistence.md.
package persistence

import (
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

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
}

// AccessPolicyRow is the durable form of a router.AccessPolicy. Match holds
// the []ClaimRule{Path,Pattern} pairs (not the compiled matcher), Grants the
// []router.Grant list, both as JSON.
type AccessPolicyRow struct {
	gorm.Model
	Name   string `gorm:"uniqueIndex"`
	Match  datatypes.JSON
	Grants datatypes.JSON
}

// FilterPolicyRow is the durable form of a router.FilterPolicy.
type FilterPolicyRow struct {
	gorm.Model
	Name       string `gorm:"uniqueIndex"`
	Match      datatypes.JSON
	MCP        string
	Tool       string
	DropFields datatypes.JSON
}
