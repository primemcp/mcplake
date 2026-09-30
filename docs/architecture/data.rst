Data Model & Request Lifecycle
==============================

.. _architecture-data-claim-rule:

Claim Rule
----------

The atomic unit of every policy. Extracts a value from the JWT claim set with
JSONPath, then tests it with a regexp.

.. code-block:: go

   type ClaimRule struct {
       Path    string // JSONPath, e.g. "$.role" or "$.groups[*]"
       Pattern string // regexp, e.g. "^db-(reader|writer)$"
   }

Evaluation (``ClaimRule.Matches(claims []byte) (bool, error)``):

1. Run ``Path`` against the decoded claim JSON.
2. If the result is a single scalar, return ``regexp.MatchString(Pattern, value)``.
3. If the result is a list (e.g. ``$.groups[*]``), return ``true`` if **any** element
   matches ``Pattern`` — this is the "is there an element in the list satisfying the
   regexp" case called out explicitly in the design brief. There is no separate code
   path for list vs. scalar rules beyond this branch; the rule shape is identical.
4. If ``Path`` resolves to nothing, treat as no match (not an error) — a missing claim
   simply fails the rule.

A ``ClaimMatcher`` is an ordered list of ``ClaimRule``\ s, ANDed together:

.. code-block:: go

   type ClaimMatcher struct {
       Rules []ClaimRule // every rule must match
   }

Access Policy
-------------

Grants a group of MCPs/tools to callers whose claims satisfy the matcher.

.. code-block:: go

   type AccessPolicy struct {
       Name  string
       Match ClaimMatcher
       Grants []Grant
   }

   type Grant struct {
       MCP   string   // registered MCP name, or "*" for all
       Tools []string // tool names within that MCP, or ["*"] for all
   }

Config shape (see ``config.example.toml``):

.. code-block:: toml

   [[access_policies]]
   name = "db-reader"
   [[access_policies.match]]
   path = "$.role"
   pattern = "^db-reader$"
   [[access_policies.grants]]
   mcp = "postgres-ro"
   tools = ["*"]

   [[access_policies]]
   name = "on-call"
   [[access_policies.match]]
   path = "$.groups[*]"
   pattern = "^oncall-.*$"
   [[access_policies.grants]]
   mcp = "postgres-rw"
   tools = ["get_user", "list_incidents"]

At request time, a call to ``(mcp, tool)`` is authorized if **any** ``AccessPolicy`` whose
``Match`` is satisfied has a ``Grant`` covering that ``(mcp, tool)``. Policies are additive —
there is no explicit deny rule in this milestone; absence of a matching grant is a
403.

Filter Policy
-------------

Same matcher, different payload: a set of field paths to drop from a specific
``(mcp, tool)`` response.

.. code-block:: go

   type FilterPolicy struct {
       Name       string
       Match      ClaimMatcher
       MCP        string
       Tool       string
       DropFields []string // field paths in the tool's own record, e.g. "$.salary"
   }

Config shape (extends today's ``filtering`` block):

.. code-block:: toml

   [[filter_policies]]
   name = "hide-pii-for-plain-users"
   mcp = "postgres-ro"
   tool = "get_user"
   drop_fields = ["$.hashed_password", "$.api_key", "$.internal_id"]
   [[filter_policies.match]]
   path = "$.role"
   pattern = "^user$"

At response time, the set of fields removed is the **union** of ``DropFields`` across
every ``FilterPolicy`` whose ``Match`` is satisfied and whose ``(MCP, Tool)`` equals the
call just made.

.. _architecture-data-mcp-registration:

MCP Registration
----------------

.. code-block:: go

   type MCPRegistration struct {
       Name      string
       Transport string // "stdio" | "sse" | "http"
       Connect   ConnectConfig // command+args for stdio, URL for sse/http
       Status    string // "connecting" | "active" | "unreachable"
       Tools     map[string]ToolSchema // populated after tools/list
   }

   type ToolSchema struct {
       Name         string
       InputSchema  json.RawMessage
       OutputSchema json.RawMessage // when the MCP provides one; nil otherwise
   }

Registration is one call regardless of trigger (startup config or the Control-Plane
API):

::

   RegisterMCP(reg MCPRegistration) error
       -> mcp.NewClient(reg.Transport, reg.Connect)
       -> client.ListTools(ctx)
       -> persistence.Upsert(reg.Name, tools)   // GORM, see below
       -> registry.refreshCache(reg.Name, tools)
       -> reg.Status = "active"

See :doc:`ADR-0003 </architecture/decisions/0003-dynamic-mcp-registration-and-schema-discovery>` for
failure handling and re-registration semantics, and
:doc:`ADR-0006 </architecture/decisions/0006-gorm-sqlite-postgres-persistence>` for the persistence
step.

Persistence Models (GORM)
-------------------------

``MCPRegistration``, ``AccessPolicy``, and ``FilterPolicy`` are the durable records behind
the in-memory shapes above. Nested rule/grant data is stored as a JSON text column so
the same struct tags work unchanged on SQLite and PostgreSQL
(:doc:`ADR-0006 </architecture/decisions/0006-gorm-sqlite-postgres-persistence>`):

.. code-block:: go

   type MCPRegistrationRow struct {
       gorm.Model
       Name      string `gorm:"uniqueIndex"`
       Transport string
       Connect   datatypes.JSON // ConnectConfig, serialized
   }

   type AccessPolicyRow struct {
       gorm.Model
       Name   string `gorm:"uniqueIndex"`
       Match  datatypes.JSON // []ClaimRule, serialized
       Grants datatypes.JSON // []Grant, serialized
   }

   type FilterPolicyRow struct {
       gorm.Model
       Name       string `gorm:"uniqueIndex"`
       Match      datatypes.JSON // []ClaimRule, serialized
       MCP        string
       Tool       string
       DropFields datatypes.JSON // []string, serialized
   }

``ToolSchema`` results from ``tools/list`` are not modeled as their own table for this
milestone — they're stored alongside the owning ``MCPRegistrationRow`` (as JSON) and
rebuilt into the in-memory ``MCPRegistry`` cache on load, since they're only ever read
as a whole per-MCP tool set, never queried individually in SQL.

At startup, ``config.toml``'s ``[[mcps]]``, ``[[access_policies]]``, and ``[[filter_policies]]``
entries are seeded into these tables by name, **once per named entry, ever**
(:doc:`ADR-0016 </architecture/decisions/0016-config-seeding-happens-once-per-entry>`): a
``seed_markers`` row per ``(kind, name)`` records that an entry has been
written, and is checked before every entry, before the caches are built.
A name with no marker is written (and then marked); a name that already
has one is left alone, however the config file now reads — the database
is the source of truth from that point on, and a restart does not
re-apply the file over a change made through the admin API. Every
Control-Plane API write instead goes through the plain upsert path and
then refreshes the corresponding in-memory cache.

.. _architecture-data-request-lifecycle:

Request Lifecycle
-----------------

This is the data-plane (fasthttp) tool-call path. Registering the MCP and defining
the policies referenced below happens beforehand, out-of-band, via the Gin
control-plane API described in :ref:`architecture-components-control-plane-api-gin`.

.. mermaid::

   sequenceDiagram
       participant C as Client
       participant GW as Gateway (fasthttp)
       participant Auth as Auth Validator
       participant Pol as Policy Engine
       participant Reg as MCP Registry
       participant M as Downstream MCP

       C->>GW: POST /v1/call {mcp, tool, args} + Bearer JWT
       GW->>Auth: ValidateToken(jwt)
       Auth-->>GW: claims | 401
       GW->>Pol: Authorize(claims, mcp, tool)
       Pol-->>GW: allowed | 403
       GW->>Reg: Disabled(mcp)?
       Reg-->>GW: yes -> 403 mcp_disabled
       GW->>Reg: Resolve(mcp)
       Reg-->>GW: mcp.Client
       GW->>M: CallTool(tool, args)
       M-->>GW: raw response JSON
       GW->>Pol: FieldsToRemove(claims, mcp, tool)
       Pol-->>GW: [field paths]
       GW->>GW: filter.Strip(response, fields)
       GW-->>C: filtered response JSON

This is the concrete form of the pipeline sketched in
:ref:`architecture-overview-pipeline-per-request`: JWT eval happens once for
authorization and is consulted again — same engine, different policy set — to decide
what the response filter strips, before the response reaches the client.

.. _architecture-data-enabledisable-gates:

Enable/disable gates
~~~~~~~~~~~~~~~~~~~~

Each of the three runtime-managed objects carries an operator ``Enabled`` flag
(default true; see :repo:`CONFIG.md <docs/CONFIG.md>` and #95). Disabling one is a
reversible kill switch, not a delete, and each acts at a different point above:

- **MCP registration disabled** — after ``Authorize`` succeeds, the gateway
  checks ``Registry.Disabled(mcp)`` and returns ``403 mcp_disabled`` (distinct
  from ``404 mcp_not_found``) without calling the downstream MCP. The
  registration, its cached tool schemas, and its live client are all kept, so
  re-enabling needs no reconnect. Every pipeline for that MCP stops.
- **Access policy disabled** — ``Engine.Authorize`` skips the policy before
  evaluating its ``Match``, so it grants nothing. A caller authorized only by a
  disabled policy gets ``403 forbidden`` and no downstream call is made — the
  pipeline never starts.
- **Filter policy disabled** — ``Engine.FieldsToRemove`` skips the policy, so
  its ``drop_fields`` do not contribute. A response that policy would have
  stripped is returned unfiltered; other enabled filter policies for the same
  ``(mcp, tool)`` still apply.
