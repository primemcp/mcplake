package persistence

import (
	"fmt"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Supported Config.Driver values.
const (
	DriverSQLite   = "sqlite"
	DriverPostgres = "postgres"
)

// defaultDriver is used when Config.Driver is unset, matching ADR-0006's
// SQLite-by-default decision for single-instance, air-gapped deployments.
const defaultDriver = DriverSQLite

// Config selects and configures the persistence backend. See
// docs/architecture/decisions/0006-gorm-sqlite-postgres-persistence.rst.
type Config struct {
	// Driver is "sqlite" (default) or "postgres".
	Driver string
	// DSN is the SQLite file path (e.g. "gateway.db") or the PostgreSQL
	// connection string, depending on Driver.
	DSN string
}

// Open opens a GORM connection per cfg and runs AutoMigrate for every model
// this package owns. SQLite (the default) uses a pure-Go driver
// (glebarez/sqlite, built on modernc.org/sqlite) so the gateway binary stays
// CGO-free, per ADR-0001/ADR-0006. Postgres connectivity is wired but not
// validated against a live server by this function.
func Open(cfg Config) (*gorm.DB, error) {
	dialector, err := dialector(cfg)
	if err != nil {
		return nil, err
	}

	db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("persistence: open %s: %w", driverName(cfg), err)
	}

	if err := db.AutoMigrate(&MCPRegistrationRow{}, &AccessPolicyRow{}, &FilterPolicyRow{}, &SeedMarkerRow{}); err != nil {
		return nil, fmt.Errorf("persistence: automigrate: %w", err)
	}

	return db, nil
}

func dialector(cfg Config) (gorm.Dialector, error) {
	switch driverName(cfg) {
	case DriverSQLite:
		return sqlite.Open(cfg.DSN), nil
	case DriverPostgres:
		return postgres.Open(cfg.DSN), nil
	default:
		return nil, fmt.Errorf("persistence: unsupported driver %q", cfg.Driver)
	}
}

func driverName(cfg Config) string {
	if cfg.Driver == "" {
		return defaultDriver
	}
	return cfg.Driver
}
