# ADR-0006: GORM for Persistence, SQLite by Default, PostgreSQL for Distributed Deployments

- Status: Accepted
- Date: 2026-09-04

## Context

Until now, `AccessPolicy`/`FilterPolicy` ([ADR-0004](0004-unified-policy-engine-for-access-and-filtering.md))
and `MCPRegistration` ([ADR-0003](0003-dynamic-mcp-registration-and-schema-discovery.md))
were described as config-file-driven (loaded from `config.yaml` at startup) plus
in-memory state. [ADR-0005](0005-use-gin-for-control-plane-api.md) now adds a
control-plane API that creates, updates, and deletes these records at runtime. That
changes the requirement:

- Runtime-created/edited policies and registrations must survive a gateway restart —
  in-memory-only state would silently lose admin changes on every deploy/crash.
- The default deployment target is a single gateway instance in an air-gapped
  environment ([`docs/OVERVIEW.md`](../../OVERVIEW.md#air-gapped-design)) — the
  default persistence choice must not require standing up an external database
  service.
- A later, explicitly anticipated requirement is running the gateway distributed
  (multiple replicas behind a load balancer, sharing one policy/registration store),
  which a single-file embedded database cannot support — multiple processes cannot
  safely share one SQLite file over a network filesystem.
- The project's existing bias is minimal dependencies and straightforward,
  reviewable code ([`docs/SCAFFOLDING.md`](../../SCAFFOLDING.md)) — the persistence
  layer should not require hand-written SQL per supported database dialect if it can
  be avoided.

## Decision

Use [GORM](https://gorm.io) as the ORM for all control-plane persistence
(`AccessPolicy`, `FilterPolicy`, `MCPRegistration`, and their nested rule/grant
data), with:

- **SQLite as the default backend** (`gorm.io/driver/sqlite` on top of a **pure-Go**
  SQLite implementation — e.g. `glebarez/sqlite` — not `mattn/go-sqlite3`, so the
  gateway binary stays CGO-free and single-binary/cross-compilable, preserving the
  deployment story established by [ADR-0001](0001-use-fasthttp-for-gateway-server.md)).
  This is what a single-instance, air-gapped deployment uses out of the box — no
  external database service required.
- **PostgreSQL as the supported backend for distributed deployments**
  (`gorm.io/driver/postgres`), selected via configuration when running multiple
  gateway replicas that must share one control-plane store. Not implemented in this
  milestone, but the schema and query patterns are constrained now (see
  Consequences) so the switch is a configuration change, not a rewrite.

Startup-time `config.yaml` entries for MCPs and policies (ADR-0003's static path) are
**upserted into the database** at boot rather than living only in memory — the
database becomes the single source of truth for the control-plane state; config.yaml
becomes a convenience bootstrap/seed mechanism for the default (SQLite,
single-instance) deployment, not a parallel state store. The in-memory Policy Engine
and MCP Registry (ADR-0003, ADR-0004) remain as read-optimized caches over this data,
loaded at startup and refreshed on every admin write — the hot tool-call path never
touches the database directly.

## Alternatives Considered

### Alternative A: Config-file-only, no database (status quo)

Advantages:
- No new dependency; nothing to operate.

Disadvantages:
- Cannot support runtime CRUD from the admin API (ADR-0005) — every change would
  require editing `config.yaml` and restarting, defeating the purpose of the admin
  API.
- No path to a distributed deployment at all.

### Alternative B: Hand-written `database/sql` with per-dialect queries

Advantages:
- No ORM "magic"; full control over generated SQL; smaller dependency footprint than
  GORM.

Disadvantages:
- The models here (`AccessPolicy`, `FilterPolicy`, `MCPRegistration` — a handful of
  straightforward CRUD entities) don't need hand-tuned SQL; hand-writing two dialects
  of every query (SQLite now, Postgres later) is pure boilerplate for this workload.
- Every future field/model change requires touching raw SQL in two places once
  Postgres support lands, instead of one struct definition.

### Alternative C: GORM, SQLite default, Postgres for distributed (chosen)

Advantages:
- One model definition, two supported dialects via GORM's driver abstraction — the
  seam the distributed-deployment requirement needs is built in, not bolted on later.
- `AutoMigrate` covers this milestone's simple, additive schema needs without a
  separate migration tool.
- Matches the explicit requirement (SQLite default, Postgres later for distributed).

Disadvantages:
- GORM's conventions (hooks, naming, soft deletes) can obscure exactly what SQL runs;
  mitigated by keeping models simple and avoiding GORM features that don't map
  predictably across SQLite and Postgres (see Consequences).
- Adds a real dependency (GORM + two driver packages) where there was none before.

### Alternative D: PostgreSQL only, from day one

Advantages:
- One backend to test and support; sidesteps any SQLite/Postgres portability
  concerns entirely.

Disadvantages:
- Forces every default, single-instance, air-gapped deployment to stand up and
  operate an external database service it doesn't otherwise need — directly against
  the project's default deployment simplicity goal. The user requirement is
  explicitly SQLite-by-default with Postgres deferred to when distribution is
  actually needed.

## Decision Criteria

- Meets the explicit requirement: SQLite by default, Postgres for distributed use
  later.
- Preserves single-binary/air-gapped default deployment (no required external
  service).
- Provides a real (not theoretical) path to a second backend without a rewrite.
- Reasonable dependency and code-complexity cost for the actual model complexity
  (a handful of CRUD entities, not a complex relational domain).

## Rationale

GORM is the smallest addition that satisfies both halves of the requirement at once:
it makes the default deployment (SQLite, embedded, no external service) as simple as
the project's other defaults, while making the anticipated Postgres/distributed path
a configuration change instead of a second implementation. Hand-written per-dialect
SQL (Alternative B) would cost more over time for models this simple, and
Postgres-only (Alternative D) breaks the default single-instance deployment story the
rest of the architecture (ADR-0001, air-gapped design) is built around.

## Consequences

### Positive

- Admin-API changes (ADR-0005) persist across restarts by default, with no external
  service required for the common single-instance case.
- A documented, low-cost path exists to PostgreSQL when a distributed deployment is
  actually needed, without redesigning the model layer.
- Config-file-driven startup (ADR-0003) and runtime admin changes now go through the
  same storage, eliminating the two-sources-of-truth risk that would exist if config
  and DB state were separate.

### Negative

- New dependency surface: GORM plus a SQLite driver plus (later) a Postgres driver.
- Schema/model design must deliberately avoid SQLite-specific or Postgres-specific
  features (e.g. Postgres native arrays/JSONB operators) to keep the dialect swap
  cheap — nested data (`ClaimRule` lists, `Grant` lists) is stored as a JSON text
  column readable by both, not a dialect-specific type, accepting that this trades
  some query-ability (can't filter on nested fields in SQL) for portability. This is
  acceptable because all matching logic runs in the ADR-0002 rule engine in Go, not
  in SQL.
- The hot tool-call path must not query the database directly (would reintroduce the
  exact per-request overhead ADR-0001 was written to avoid); this makes the
  in-memory-cache-refreshed-on-write pattern a hard requirement, not an optimization.

### Risks

- SQLite has no built-in replication and limited write concurrency — acceptable for
  a single-instance control plane where writes are infrequent admin operations, but
  this is precisely why Postgres, not "scale up SQLite," is the documented answer for
  distributed deployments.
- If the in-memory cache refresh (on admin write) is implemented incorrectly, the
  data plane could serve stale policy decisions after an admin change. Mitigated by
  the integration test in ADR-0005's Validation section, which asserts immediate
  visibility.
- `AutoMigrate` is adequate for this milestone's additive schema changes but does not
  handle destructive migrations (column removal/renames) safely; flagged as
  follow-up before the schema stabilizes for a 1.0.

### Follow-up

- Introduce a proper migration tool (e.g. `golang-migrate` or GORM's migration
  helpers beyond `AutoMigrate`) before the schema needs a destructive change.
- When Postgres support is actually implemented, add it to CI as a second test
  target (e.g. via a container) so both dialects are continuously verified, not just
  SQLite.
- Document the two-backend configuration (`persistence.driver: sqlite|postgres`) in
  `config.example.yaml` and `docs/CONFIG.md`.

## Validation

Run the full model test suite (`AccessPolicy`/`FilterPolicy`/`MCPRegistration` CRUD,
plus the ADR-0005 integration test) against SQLite now; re-run the same suite against
a Postgres test container once that driver is added, to confirm no SQLite-specific
behavior leaked into the model layer or query patterns.

## References

- [ADR-0003](0003-dynamic-mcp-registration-and-schema-discovery.md) — registration
  data now persisted here.
- [ADR-0004](0004-unified-policy-engine-for-access-and-filtering.md) — policy data
  now persisted here.
- [ADR-0005](0005-use-gin-for-control-plane-api.md) — the API that writes through
  this layer.
- [data.md](../data.md) — updated model shapes.
- [`docs/OVERVIEW.md`](../../OVERVIEW.md#air-gapped-design) — default deployment
  constraint this ADR preserves.
