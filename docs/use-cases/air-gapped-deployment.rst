Air-Gapped Deployment
=====================

The problem
-----------

You want agents inside a network with no internet access to use MCP servers
that run inside it too — with the same identity and access control as
everything else there, and without any component calling out.

What the gateway needs at runtime
---------------------------------

.. list-table::
   :header-rows: 1
   :widths: 30 70

   * - Dependency
     - In an air-gapped network
   * - OIDC provider
     - An internal one (Keycloak, Dex, AD FS, …). The gateway fetches its JWKS
       from ``oidc.jwks_url`` and caches it; nothing else is contacted.
   * - Persistence
     - Embedded SQLite by default — a file next to the gateway. Or an internal
       PostgreSQL.
   * - MCP servers
     - ``stdio`` servers run on the gateway host; ``http``/``sse`` servers
       anywhere on the internal network (over ``https``, or ``http`` on
       loopback).
   * - Everything else
     - Nothing. No telemetry, no update checks, no license server.

The gateway makes no outbound connections of its own except to the JWKS URL
and the MCP servers you register. Even the admin UI's sign-in endpoints are
configured explicitly (``[admin_auth.login]``) rather than discovered at
startup.

Building for the enclave
------------------------

Build on a connected machine and carry the artifacts across — the build needs
Go modules and the admin UI's JavaScript packages, the runtime does not.

.. code-block:: bash

   # On a connected machine
   CGO_ENABLED=0 make build              # static cmd/gateway/mcp-gateway, UI embedded
   docker build -f deploy/webui/Dockerfile -t mcplake-webui .   # optional
   docker save mcplake-webui -o mcplake-webui.tar

With ``CGO_ENABLED=0`` the gateway binary is statically linked and has no
runtime dependencies — not even the C library, so it runs on any Linux host of
the same architecture. (Without it, Go links against the build machine's
glibc.) Transfer it (and the image, if you use it) through your normal import process.

If you must build inside the enclave, mirror the dependencies first:
``go work vendor`` at the repository root (or an internal Go module proxy
via ``GOPROXY``), and an internal npm registry for the UI's ``bun install``.

Packaging MCP servers
---------------------

A ``stdio`` server's runtime and packages must be installed on the gateway
host — for example a Python virtual environment built from a wheelhouse:

.. code-block:: bash

   # connected machine
   pip download -r requirements.txt -d wheels/
   # enclave
   python3 -m venv /opt/mcp/directory
   /opt/mcp/directory/bin/pip install --no-index --find-links wheels/ -r requirements.txt

then register it with an absolute ``command``:

.. code-block:: toml

   [[mcps]]
   name = "directory"
   type = "stdio"
   command = "/opt/mcp/directory/bin/python"
   arguments = ["/opt/mcp/directory/server.py"]

The repository's demo image does the same at build time
(:repo:`deploy/demo/Dockerfile`), precisely so the container needs no network
at start.

Configuration
-------------

.. code-block:: toml

   [server]
   data_plane_addr = ":8080"
   control_plane_addr = "10.0.5.10:8081"     # management network only

   [oidc]
   jwks_url = "https://sso.corp.internal/realms/main/protocol/openid-connect/certs"
   issuer = "https://sso.corp.internal/realms/main"
   audience = "mcplake"
   jwks_cache_ttl = "6h"

   [persistence]
   driver = "sqlite"
   dsn = "/var/lib/mcplake/gateway.db"

   [[admin_auth.match]]
   path = "$.realm_access.roles[*]"
   pattern = "^mcplake-admin$"

- The JWKS, admin login endpoints and network MCP servers must use ``https``
  (or ``http`` on loopback): issue them certificates from your internal CA and
  make sure the gateway host trusts it.
- A longer ``jwks_cache_ttl`` keeps the gateway validating tokens through a
  short identity-provider outage; keep it shorter than your key-rotation
  overlap.

Known caveat
------------

The admin web UI requests its typeface (IBM Plex) from Google Fonts. Offline
the request fails and browsers fall back to system fonts; the UI is otherwise
fully functional.

See also
--------

- :doc:`/getting-started/installation`
- :doc:`/concepts/authentication` — TLS and the trust model.
