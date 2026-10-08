package persistence

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Seedable kinds. They name the three config arrays Config.Seed walks, and
// are stored verbatim, so changing one of these strings orphans existing
// markers and would re-seed every entry of that kind once.
const (
	SeedKindMCP          = "mcp"
	SeedKindAccessPolicy = "access_policy"
	SeedKindFilterPolicy = "filter_policy"
)

// SeedMarkerRow records that one named config entry has been seeded into
// the store. Its whole purpose is to make seeding happen exactly once per
// entry, ever: without it a restart re-applies the config file over
// whatever the operator has since done through the admin API, silently
// reverting a disable or resurrecting a deleted policy. See ADR-0016.
//
// The marker is deliberately separate from the rows it describes, because
// it must outlive them — a deleted policy leaves no row behind, and the
// marker is the only thing that stops the next boot from recreating it.
type SeedMarkerRow struct {
	Kind     string `gorm:"primaryKey;size:32"`
	Name     string `gorm:"primaryKey;size:255"`
	SeededAt time.Time
}

func (SeedMarkerRow) TableName() string { return "seed_markers" }

// SeedMarkerRepo records and reports which config entries have been seeded.
type SeedMarkerRepo struct {
	db *gorm.DB
}

// NewSeedMarkerRepo returns a repository backed by db. db is expected to
// already have run AutoMigrate (see Open).
func NewSeedMarkerRepo(db *gorm.DB) *SeedMarkerRepo {
	return &SeedMarkerRepo{db: db}
}

// seedKey is the in-memory form of a marker's composite key. Using a struct
// rather than a joined string keeps a name containing the separator from
// colliding with another entry.
type seedKey struct {
	Kind string
	Name string
}

// SeededSet is the set of entries already seeded, as returned by Seeded.
type SeededSet map[seedKey]struct{}

// Has reports whether the named entry of that kind has ever been seeded.
func (s SeededSet) Has(kind, name string) bool {
	_, ok := s[seedKey{Kind: kind, Name: name}]
	return ok
}

// Seeded loads every marker in one query, so seeding does not issue a
// round trip per config entry.
func (r *SeedMarkerRepo) Seeded(ctx context.Context) (SeededSet, error) {
	var rows []SeedMarkerRow
	if err := r.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("persistence: list seed markers: %w", err)
	}
	set := make(SeededSet, len(rows))
	for _, row := range rows {
		set[seedKey{Kind: row.Kind, Name: row.Name}] = struct{}{}
	}
	return set, nil
}

// Mark records that the named entry has been seeded. It is idempotent, so a
// crash between writing an entry and marking it costs at most one
// re-seed of that entry on the next boot rather than a failed startup.
func (r *SeedMarkerRepo) Mark(ctx context.Context, kind, name string) error {
	row := SeedMarkerRow{Kind: kind, Name: name, SeededAt: time.Now().UTC()}
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "kind"}, {Name: "name"}},
			DoUpdates: clause.AssignmentColumns([]string{"seeded_at"}),
		}).
		Create(&row).Error
	if err != nil {
		return fmt.Errorf("persistence: mark %s %q seeded: %w", kind, name, err)
	}
	return nil
}
