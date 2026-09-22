package config

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/atsokha/mcplake/cache"
	"github.com/atsokha/mcplake/persistence"
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
// it skips so the operator can see why their edit had no effect.
//
// Seeded MCPs start in cache.StatusConnecting: Seed only records the
// declared intent to register each MCP, it does not connect to any of them
// -- that is Registry.Register's job (see cache.Registry.RegisterAll),
// performed separately after seeding.
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

	// mark is called only after the entry itself is written, so a crash in
	// between costs one harmless re-seed rather than losing the entry.
	seedOnce := func(kind, name string, write func() error) error {
		if seeded.Has(kind, name) {
			slog.Debug("config entry already seeded; the stored record wins",
				"kind", kind, "name", name)
			return nil
		}
		if err := write(); err != nil {
			return err
		}
		return markerRepo.Mark(ctx, kind, name)
	}

	for _, reg := range c.MCPRegistrations() {
		reg.Status = cache.StatusConnecting
		err := seedOnce(persistence.SeedKindMCP, reg.Name, func() error {
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
		err := seedOnce(persistence.SeedKindAccessPolicy, p.Name, func() error {
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
		err := seedOnce(persistence.SeedKindFilterPolicy, p.Name, func() error {
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
