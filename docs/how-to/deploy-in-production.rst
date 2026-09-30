Deploy in Production
====================

A checklist for running the gateway beyond a laptop, and the pieces that go
around it.

Checklist
---------

.. list-table::
   :widths: 5 95

   * - ☐
     - ``admin_auth`` configured and ``control_plane_addr`` on a private
       interface — :doc:`secure-the-control-plane`.
   * - ☐
     - TLS terminated in front of both listeners (below).
   * - ☐
     - ``[oidc]`` points at your production provider; ``audience`` is specific
       to the gateway.
   * - ☐
     - Persistence on a path or database that is backed up —
       :doc:`upgrade-and-back-up`.
   * - ☐
     - The gateway runs as an unprivileged user that can start your ``stdio``
       servers and nothing more.
   * - ☐
     - Every MCP server is registered with the narrowest credentials it needs.
   * - ☐
     - No access policy with an empty ``match`` or ``mcp = "*"`` except for
       administrators.

Run it as a service
-------------------

The gateway is one binary with one flag. With systemd:

.. code-block:: ini

   # /etc/systemd/system/mcplake.service
   [Unit]
   Description=mcplake MCP gateway
   After=network-online.target
   Wants=network-online.target

   [Service]
   User=mcplake
   Group=mcplake
   WorkingDirectory=/var/lib/mcplake
   ExecStart=/usr/local/bin/mcp-gateway --config /etc/mcplake/config.toml
   Restart=on-failure
   NoNewPrivileges=true
   ProtectSystem=strict
   ReadWritePaths=/var/lib/mcplake
   PrivateTmp=true

   [Install]
   WantedBy=multi-user.target

Adjust ``ReadWritePaths`` (and add ``ReadOnlyPaths`` as needed) for what your
``stdio`` MCP servers must reach — they run as children of this service and
inherit its sandbox. The gateway logs to standard error; ``journalctl -u
mcplake`` shows it. ``SIGTERM`` shuts it down cleanly.

Put TLS in front
----------------

The gateway does not terminate TLS. Put a reverse proxy or load balancer in
front of it, and keep the hop behind it private. Two things matter for MCP
traffic:

- **Do not buffer responses** on ``/v1/sse`` and ``/v1/mcp``: both stream
  (Server-Sent Events).
- **Allow long-lived connections**: an SSE stream stays open for the whole
  session.

With nginx:

.. code-block:: nginx

   server {
       listen 443 ssl;
       server_name mcp.example.com;
       ssl_certificate     /etc/ssl/mcp.example.com.crt;
       ssl_certificate_key /etc/ssl/mcp.example.com.key;

       location / {
           proxy_pass http://127.0.0.1:8080;
           proxy_http_version 1.1;
           proxy_set_header Connection "";
           proxy_set_header Host $host;
           proxy_buffering off;
           proxy_read_timeout 1h;
       }
   }

Serve the control plane (and the admin UI) on a separate, internal-only
server block or host.

Containers
----------

The project does not publish images yet. :repo:`compose.yaml` and
:repo:`deploy/` show a complete containerized layout — gateway, separate admin
UI container, PostgreSQL, network MCP servers — and are the best starting
point for your own images:

- :repo:`deploy/demo/Dockerfile` builds the UI and the gateway in its first two
  stages; its runtime stage is Python-based only because the demo runs a
  Python MCP server over stdio. Choose a runtime base that contains what your
  own ``stdio`` servers need, or a minimal one if all your servers are
  network (``http``/``sse``) servers.
- :repo:`deploy/webui/Dockerfile` is the admin UI container, suitable as is
  (see :ref:`reference-cli-webui` for its settings).

SELinux hosts
~~~~~~~~~~~~~

On SELinux-enforcing hosts (Fedora, RHEL and derivatives) containers are
denied access to bind-mounted files unless the mount is relabelled. Use
``:Z`` for a mount used by one container and ``:z`` for one shared by several
— the demo's ``compose.yaml`` does this for its configuration and MCP server
mounts.

Observability
-------------

- **Logs** are structured (``key=value``) on standard error. Warnings to act
  on include: an unauthenticated control plane, a configuration entry that
  differs from its stored record, a filter policy that matched and removed
  nothing, and an MCP server that cannot be reached.
- **Health**: ``GET /healthz`` on the data plane and ``GET /admin/healthz`` on
  the control plane answer without authentication.
- **MCP server status** is available from ``GET /admin/mcps``.
