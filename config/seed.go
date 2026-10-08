package config

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/primemcp/mcplake/cache"
	"github.com/primemcp/mcplake/persistence"
	"github.com/primemcp/mcplake/router"
)

// Seed writes c's mcps/access_policies/filter_policies entries into the
// given persistence repositories, **once per named entry, ever**. Per
// ADR-0006 the config file is a seed mechanism and the database is the
// source of truth from then on; ADR-0016 makes that literally true.
//
// Each entry is seeded the first time its name is seen and never again:
// a marker recorded in the store (persistence.SeedMarkerRepo) survives both
// the entry and its deletion. Without that, every restart re-applied the
// config file over whatever the operator had since done through the admin
// API -- silently re-enabling an MCP they had disabled, resurrecting a
// policy they had deleted, and reverting a drop_fields list they had
// tightened. A revocation that does not survive a restart is not a
// revocation.
//
// Adding a new entry to the config file still works: its name has no marker,
// so the next boot seeds it. Editing an entry that has already been seeded
// does not -- the stored record is authoritative, and Seed logs each entry
// it skips so the operator can see why their edit had no effect. A skip
// whose stored record still says exactly what the config file says is
// logged at DEBUG (the ordinary case on every boot after the first); a skip
// where the two disagree is logged at WARN, naming the fields that differ,
// because otherwise an operator who edits a seeded entry and restarts is
// told nothing at all and has no way to tell that their edit was ignored.
//
// Seeded MCPs start in cache.StatusConnecting: Seed only records the
// declared intent to register each MCP, it does not connect to any of them
// -- that is Registry.Register's job (see cache.Registry.RegisterAll),
// performed separately after seeding.
//
// The very first call against a store that predates seed_markers (an
// upgrade from a version without ADR-0016) backfills a marker for every
// row that already exists, before seeding anything -- see
// backfillSeedMarkers. Without that, the first post-upgrade boot would
// re-apply config over an existing row exactly once, silently reverting
// whatever an operator had changed through the admin API on it (a
// disable, a tightened drop_fields list) on the one boot meant to install
// the fix for that. A name with NO row -- a policy the operator deleted
// before upgrading -- has nothing to backfill from and is still seeded
// fresh on that first boot, same as ADR-0016 already documents; there is
// no historical record of "this was deleted" to recover once the row is
// gone and no marker ever existed for it.
func (c *Config) Seed(
	ctx context.Context,
	markerRepo *persistence.SeedMarkerRepo,
	mcpRepo *persistence.MCPRegistrationRepo,
	accessRepo *persistence.AccessPolicyRepo,
	filterRepo *persistence.FilterPolicyRepo,
) error {
	seeded, err := markerRepo.Seeded(ctx)
	if err != nil {
		return fmt.Errorf("config: read seed markers: %w", err)
	}

	stored, err := loadStoredRecords(ctx, mcpRepo, accessRepo, filterRepo)
	if err != nil {
		return fmt.Errorf("config: read stored records: %w", err)
	}

	seeded, err = backfillSeedMarkers(ctx, markerRepo, stored, seeded)
	if err != nil {
		return fmt.Errorf("config: backfill seed markers: %w", err)
	}

	// mark is called only after the entry itself is written, so a crash in
	// between costs one harmless re-seed rather than losing the entry.
	//
	// reportSkip is invoked instead of the write when the entry is already
	// seeded; each caller supplies it because only the caller knows how to
	// compare its own kind of record against what the store holds.
	seedOnce := func(kind, name string, reportSkip func(), write func() error) error {
		if seeded.Has(kind, name) {
			reportSkip()
			return nil
		}
		if err := write(); err != nil {
			return err
		}
		if err := markerRepo.Mark(ctx, kind, name); err != nil {
			return fmt.Errorf("config: mark %s %q seeded: %w", kind, name, err)
		}
		return nil
	}

	for _, reg := range c.MCPRegistrations() {
		reg.Status = cache.StatusConnecting
		err := seedOnce(persistence.SeedKindMCP, reg.Name,
			func() {
				got, ok := stored.mcps[reg.Name]
				reportSeedSkipped(persistence.SeedKindMCP, reg.Name, ok,
					mcpSeedFields(reg), mcpSeedFields(got))
			},
			func() error {
				if err := mcpRepo.Upsert(ctx, reg); err != nil {
					return fmt.Errorf("config: seed mcp %q: %w", reg.Name, err)
				}
				return nil
			})
		if err != nil {
			return err
		}
	}

	accessPolicies, err := c.RouterAccessPolicies()
	if err != nil {
		return fmt.Errorf("config: seed access policies: %w", err)
	}
	for _, p := range accessPolicies {
		err := seedOnce(persistence.SeedKindAccessPolicy, p.Name,
			func() {
				got, ok := stored.accessPolicies[p.Name]
				reportSeedSkipped(persistence.SeedKindAccessPolicy, p.Name, ok,
					accessPolicySeedFields(p), accessPolicySeedFields(got))
			},
			func() error {
				if err := accessRepo.Upsert(ctx, p); err != nil {
					return fmt.Errorf("config: seed access policy %q: %w", p.Name, err)
				}
				return nil
			})
		if err != nil {
			return err
		}
	}

	filterPolicies, err := c.RouterFilterPolicies()
	if err != nil {
		return fmt.Errorf("config: seed filter policies: %w", err)
	}
	for _, p := range filterPolicies {
		err := seedOnce(persistence.SeedKindFilterPolicy, p.Name,
			func() {
				got, ok := stored.filterPolicies[p.Name]
				reportSeedSkipped(persistence.SeedKindFilterPolicy, p.Name, ok,
					filterPolicySeedFields(p), filterPolicySeedFields(got))
			},
			func() error {
				if err := filterRepo.Upsert(ctx, p); err != nil {
					return fmt.Errorf("config: seed filter policy %q: %w", p.Name, err)
				}
				return nil
			})
		if err != nil {
			return err
		}
	}

	return nil
}

// storedRecords is everything the store already held when Seed began, keyed
// by name. Seed needs it twice -- backfillSeedMarkers needs to know which
// names exist, and a skipped entry has to be compared against what is
// actually stored under its name -- so it is read once, here, rather than
// once per use.
type storedRecords struct {
	mcps           map[string]cache.MCPRegistration
	accessPolicies map[string]router.AccessPolicy
	filterPolicies map[string]router.FilterPolicy
}

func loadStoredRecords(
	ctx context.Context,
	mcpRepo *persistence.MCPRegistrationRepo,
	accessRepo *persistence.AccessPolicyRepo,
	filterRepo *persistence.FilterPolicyRepo,
) (storedRecords, error) {
	var stored storedRecords

	mcps, err := mcpRepo.List(ctx)
	if err != nil {
		return stored, fmt.Errorf("list mcps: %w", err)
	}
	stored.mcps = make(map[string]cache.MCPRegistration, len(mcps))
	for _, reg := range mcps {
		stored.mcps[reg.Name] = reg
	}

	accessPolicies, err := accessRepo.List(ctx)
	if err != nil {
		return stored, fmt.Errorf("list access policies: %w", err)
	}
	stored.accessPolicies = make(map[string]router.AccessPolicy, len(accessPolicies))
	for _, p := range accessPolicies {
		stored.accessPolicies[p.Name] = p
	}

	filterPolicies, err := filterRepo.List(ctx)
	if err != nil {
		return stored, fmt.Errorf("list filter policies: %w", err)
	}
	stored.filterPolicies = make(map[string]router.FilterPolicy, len(filterPolicies))
	for _, p := range filterPolicies {
		stored.filterPolicies[p.Name] = p
	}

	return stored, nil
}

// reportSeedSkipped logs why an already-seeded config entry had no effect.
//
// At DEBUG when the stored record says exactly what the config file says --
// the ordinary case on every boot after the first, and not worth an
// operator's attention -- and also when no row exists at all under that
// name, which means the operator deleted it through the admin API and
// ADR-0016's marker is doing precisely the job it exists for.
//
// At WARN when the two disagree. ADR-0016 lists exactly this as its open
// follow-up ("surface 'config entry skipped because it is already seeded'"),
// and DEBUG is below the default level, so until this existed an operator
// who edited a seeded entry and restarted was told nothing at all. Only the
// names of the mismatched fields are logged -- see seedField on why not the
// values.
//
// This changes nothing about what Seed writes. The stored record still wins
// unconditionally; ADR-0016's revocation-durability guarantees are untouched.
func reportSeedSkipped(kind, name string, storedExists bool, fromConfig, fromStore []seedField) {
	if !storedExists {
		slog.Debug("config entry already seeded and since deleted; it is not recreated",
			"kind", kind, "name", name)
		return
	}
	differing := seedFieldsDiffer(fromConfig, fromStore)
	if len(differing) == 0 {
		slog.Debug("config entry already seeded; the stored record wins",
			"kind", kind, "name", name)
		return
	}
	slog.Warn("config entry already seeded and DIFFERS from the stored record; "+
		"the stored record is in force and this part of the config file has no effect "+
		"(change it through the admin API, or delete the stored entry first)",
		"kind", kind, "name", name, "differing_fields", differing)
}

// backfillSeedMarkers marks every already-stored MCP/access-policy/filter-policy
// row as seeded, for any row that predates the seed_markers table and
// therefore has no marker yet. It never writes to the rows themselves --
// only records that they exist -- so a value an operator changed after the
// original (marker-less) seed stays exactly as they left it. Returns the
// possibly-refreshed seeded set: re-read from the store only when this call
// actually backfilled something, so a normal boot (nothing to backfill)
// costs nothing beyond the reads loadStoredRecords already did.
func backfillSeedMarkers(
	ctx context.Context,
	markerRepo *persistence.SeedMarkerRepo,
	stored storedRecords,
	seeded persistence.SeededSet,
) (persistence.SeededSet, error) {
	backfilledAny := false

	mark := func(kind, name string) error {
		if seeded.Has(kind, name) {
			return nil
		}
		if err := markerRepo.Mark(ctx, kind, name); err != nil {
			return fmt.Errorf("mark %s %q seeded: %w", kind, name, err)
		}
		backfilledAny = true
		return nil
	}

	for name := range stored.mcps {
		if err := mark(persistence.SeedKindMCP, name); err != nil {
			return nil, err
		}
	}
	for name := range stored.accessPolicies {
		if err := mark(persistence.SeedKindAccessPolicy, name); err != nil {
			return nil, err
		}
	}
	for name := range stored.filterPolicies {
		if err := mark(persistence.SeedKindFilterPolicy, name); err != nil {
			return nil, err
		}
	}

	if !backfilledAny {
		return seeded, nil
	}
	refreshed, err := markerRepo.Seeded(ctx)
	if err != nil {
		return nil, fmt.Errorf("re-read seed markers after backfill: %w", err)
	}
	return refreshed, nil
}
