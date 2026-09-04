package config

import (
	"context"
	"fmt"

	"github.com/atsokha/mcplake/cache"
	"github.com/atsokha/mcplake/persistence"
)

// Seed upserts c's mcps/access_policies/filter_policies entries into the
// given persistence repositories, by name. Per ADR-0006, this is a boot-time
// reconciliation, run before the in-memory Registry/Policy Engine caches are
// built: config.yaml becomes a seed mechanism, and the database is the
// source of truth from then on. Running Seed again with the same Config
// against the same repositories is idempotent — each entry is upserted by
// name, so no duplicate rows are created (see the repositories' own Upsert
// semantics), and a changed entry is reflected as an update.
//
// Seeded MCPs start in cache.StatusConnecting: Seed only records the
// declared intent to register each MCP, it does not connect to any of them
// — that is Registry.Register's job (see cache.Registry.RegisterAll),
// performed separately after seeding.
func (c *Config) Seed(ctx context.Context, mcpRepo *persistence.MCPRegistrationRepo, accessRepo *persistence.AccessPolicyRepo, filterRepo *persistence.FilterPolicyRepo) error {
	for _, reg := range c.MCPRegistrations() {
		reg.Status = cache.StatusConnecting
		if err := mcpRepo.Upsert(ctx, reg); err != nil {
			return fmt.Errorf("config: seed mcp %q: %w", reg.Name, err)
		}
	}

	accessPolicies, err := c.RouterAccessPolicies()
	if err != nil {
		return fmt.Errorf("config: seed access policies: %w", err)
	}
	for _, p := range accessPolicies {
		if err := accessRepo.Upsert(ctx, p); err != nil {
			return fmt.Errorf("config: seed access policy %q: %w", p.Name, err)
		}
	}

	filterPolicies, err := c.RouterFilterPolicies()
	if err != nil {
		return fmt.Errorf("config: seed filter policies: %w", err)
	}
	for _, p := range filterPolicies {
		if err := filterRepo.Upsert(ctx, p); err != nil {
			return fmt.Errorf("config: seed filter policy %q: %w", p.Name, err)
		}
	}

	return nil
}
