Architecture Overview
=====================

This document describes the current architectural direction for the MCP Gateway,
superseding the high-level sketch in :doc:`/architecture/design-history` with the
concrete pipeline being built for Phase 1. Component detail lives in
:doc:`/architecture/components`; data structures and request/response flow live in
:doc:`/architecture/data`; trust boundaries and what is authenticated/authorized where
live in :doc:`/architecture/security`. Individual technology and pattern choices are
recorded as ADRs in :doc:`/architecture/decisions/index`.

System Boundary
---------------

.. mermaid::

   flowchart LR
       Agent[AI Agent / Client] -- "MCP over Streamable HTTP / SSE,\nor POST /v1/call\n(data plane, Bearer JWT)" --> DP["fasthttp\ntool-call proxy\n+ MCP server"]
       Admin[Operator / Admin UI] -- "CRUD: MCPs, policies\n(control plane)" --> CP["Gin\nadmin API"]
       DP -- "reads (cache)" --> Store[(SQLite / PostgreSQL\nvia GORM)]
       CP -- "writes + refreshes cache" --> Store
       DP -- "stdio / SSE / HTTP" --> MCP1[Downstream MCP A]
       DP -- "stdio / SSE / HTTP" --> MCP2[Downstream MCP B]
       DP -- "stdio / SSE / HTTP" --> MCP3[Downstream MCP N]
       OIDC[OIDC Provider] -. "JWKS" .-> DP

The gateway is the single trust boundary between callers and downstream MCP servers.
Callers never talk to an MCP directly. The gateway is the only component that knows
which MCPs exist, what they can do, and what a given caller is allowed to see.

The gateway exposes **two separate HTTP surfaces**, each built on the stack suited to
its workload (:doc:`ADR-0001 </architecture/decisions/0001-use-fasthttp-for-gateway-server>`,
:doc:`ADR-0005 </architecture/decisions/0005-use-gin-for-control-plane-api>`):

- **Data plane** (``fasthttp``) — the performance-critical tool-call proxy, read-only
  against the policy/registration store via an in-memory cache. It speaks MCP
  itself, at ``/v1/mcp`` (Streamable HTTP) and ``/v1/sse``, so an off-the-shelf MCP
  client can connect to the gateway directly; ``POST /v1/call`` remains for callers
  that prefer plain REST. Both run the same pipeline
  (:doc:`ADR-0021 </architecture/decisions/0021-serve-mcp-on-the-data-plane>`).
- **Control plane** (``Gin``) — low-volume CRUD for MCP registrations and access/filter
  policies, backed by SQLite by default and PostgreSQL for distributed deployments
  (:doc:`ADR-0006 </architecture/decisions/0006-gorm-sqlite-postgres-persistence>`).

The two surfaces never share a request path; they only share the persistence layer
underneath them.

Goal for This Milestone
-----------------------

1. A data-plane HTTP gateway (built on ``fasthttp`` — see :doc:`ADR-0001 </architecture/decisions/0001-use-fasthttp-for-gateway-server>`)
   that terminates tool-call requests, evaluates the caller's JWT, and routes to the
   correct downstream MCP.
2. A control-plane admin API (built on ``Gin`` — see :doc:`ADR-0005 </architecture/decisions/0005-use-gin-for-control-plane-api>`)
   so downstream MCPs and access/filter policies can be registered and managed at
   runtime, backed by persistent storage (SQLite by default, PostgreSQL for
   distributed deployments — see :doc:`ADR-0006 </architecture/decisions/0006-gorm-sqlite-postgres-persistence>`).
   On registration, an MCP's tools and I/O schemas are fetched and cached
   (see :doc:`ADR-0003 </architecture/decisions/0003-dynamic-mcp-registration-and-schema-discovery>`).
   The admin API is JWT-authenticated and claim-gated when ``admin_auth`` is
   configured (see :doc:`ADR-0010 </architecture/decisions/0010-control-plane-admin-authentication>`),
   and can additionally be exposed as MCP tools for MCP-speaking operators
   (see :doc:`ADR-0011 </architecture/decisions/0011-mcp-control-server>`).
3. An MCP-protocol data-plane surface, so the gateway is reachable by any MCP client
   and its ``tools/list`` is filtered per caller to exactly the tools that caller's
   access policy grants (see :doc:`ADR-0021 </architecture/decisions/0021-serve-mcp-on-the-data-plane>`).
4. A single policy pipeline that runs the same JWT-claim rule evaluation twice per
   request — once to authorize which MCPs/tools a caller may invoke, and once to decide
   which response fields must be dropped before the result reaches the caller
   (see :doc:`ADR-0002 </architecture/decisions/0002-jsonpath-regexp-claim-rule-engine>` and
   :doc:`ADR-0004 </architecture/decisions/0004-unified-policy-engine-for-access-and-filtering>`).

.. _architecture-overview-pipeline-per-request:

Pipeline (per request)
----------------------

.. mermaid::

   flowchart LR
       A["JWT eval\n(auth + access policy)"] --> G{"mcp\nenabled?"}
       G -->|no| X["403 mcp_disabled"]
       G -->|yes| B["mcps\n(allowed MCP set)"]
       B --> C["tools\n(allowed tool set)"]
       C --> D["tool call\n(MCP client)"]
       D --> E["resp_filter\n(claims-driven field drop)"]
       E -.->|"same rule engine,\nre-evaluated against\nresponse-filter policies"| A
       E --> F[Client response]

Read as: a JWT is evaluated once to authorize a ``(mcp, tool)`` pair; the call executes;
the response is passed back through the same claim-rule engine — this time matched
against filter policies instead of access policies — before it leaves the gateway. Full
sequence and payload shapes are in :ref:`architecture-data-request-lifecycle`.

Each of MCP registrations, access policies, and filter policies carries an operator
``Enabled`` flag (default true). Disabling one is a reversible kill switch, not a delete:
a disabled MCP rejects every call with ``403 mcp_disabled`` while staying connected; a
disabled access policy grants nothing (so the pipeline never starts); a disabled filter
policy strips nothing (so the response is returned unfiltered). See
:doc:`ADR-0009 </architecture/decisions/0009-operator-enable-disable-flag>`,
:ref:`architecture-data-enabledisable-gates`, and :doc:`/reference/configuration`.

.. _architecture-overview-non-goals-for-this-milestone:

Non-Goals for This Milestone
----------------------------

- Argument-level request validation against input schemas (Phase 2).
- Pluggable/scripted transforms beyond field-drop filtering (Phase 2).
- Audit trail persistence and distributed tracing (Phase 3).
- Multi-gateway federation.

These remain listed in :ref:`architecture-design-history-future-considerations` and
are unchanged by this milestone.
