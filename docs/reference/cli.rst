.. _reference-cli:

Command-Line Reference
======================

mcplake ships two executables. Neither reads environment variables for its
own settings except where listed below.

.. _reference-cli-gateway:

``mcp-gateway``
---------------

The gateway: data plane, control plane and (optionally) the admin MCP
server, all configured by one TOML file. Built by ``make build`` into
``cmd/gateway/mcp-gateway``, or with ``go build ./cmd/gateway``.

.. code-block:: console

   $ mcp-gateway --config /etc/mcplake/config.toml

.. list-table::
   :header-rows: 1
   :widths: 20 20 60

   * - Flag
     - Default
     - Description
   * - ``--config``
     - ``config.toml``
     - Path to the TOML configuration file. See :doc:`configuration`.

The gateway reads no environment variables of its own: every setting comes
from the config file. The environment matters in one place only — a
``stdio`` MCP subprocess starts with a minimal base environment taken from the
gateway's, plus the entry's own ``env`` table
(:doc:`ADR-0018 </architecture/decisions/0018-stdio-mcp-subprocesses-get-a-minimal-base-environment>`).

``SIGINT`` and ``SIGTERM`` shut the gateway down gracefully. A configuration
or initialization error exits with status 1 and a message prefixed
``mcplake:``.

.. _reference-cli-webui:

``webui``
---------

A small static-file server for the admin web UI, used by the ``webui``
container (:doc:`ADR-0020 </architecture/decisions/0020-serve-the-admin-ui-from-its-own-container>`).
It serves the built single-page app and reverse-proxies API calls to the
gateway's control plane, so the browser talks to one origin. Its defaults
suit the container image (:repo:`deploy/webui/Dockerfile`).

Every flag can also be set by an environment variable; an explicit flag
wins.

.. list-table::
   :header-rows: 1
   :widths: 18 22 20 40

   * - Flag
     - Environment variable
     - Default
     - Description
   * - ``--addr``
     - ``WEBUI_ADDR``
     - ``:8081``
     - Listen address.
   * - ``--root``
     - ``WEBUI_ROOT``
     - ``/srv/www``
     - Directory holding the built SPA.
   * - ``--api-prefix``
     - ``WEBUI_API_PREFIX``
     - ``/admin``
     - Path prefix reverse-proxied to the gateway control plane.
   * - ``--api-target``
     - ``WEBUI_API_TARGET``
     - *(empty)*
     - Control-plane base URL, e.g. ``http://gateway:8081``. Empty disables
       proxying.
