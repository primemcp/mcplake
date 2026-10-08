Secure the Control Plane
========================

The control plane can register ``stdio`` MCP servers — that is, run commands
on the gateway host — and it returns registrations including their ``env``
values, which often hold credentials. Secure it before anything else.

1. Keep it off the public network
---------------------------------

Bind ``control_plane_addr`` to loopback or a management interface, never to
the interface agents use:

.. code-block:: toml

   [server]
   data_plane_addr = "0.0.0.0:8080"
   control_plane_addr = "127.0.0.1:8081"   # or a management-network address

2. Require an administrator token
---------------------------------

Add claim rules that only administrators' tokens satisfy:

.. code-block:: toml

   [[admin_auth.match]]
   path = "$.realm_access.roles[*]"
   pattern = "^mcplake-admin$"

Every ``/admin/*`` request except ``GET /admin/healthz`` and
``GET /admin/auth/config`` then needs ``Authorization: Bearer <token>`` —
validated like any data-plane token — whose claims satisfy **all** the rules.
Otherwise the answer is ``401`` (no or invalid token) or ``403`` (valid token,
not an administrator).

Pick a claim that only administrators' tokens can carry. A rule such as
``$.email`` matching ``@example\.com$`` would make every employee an
administrator.

Restart the gateway; the startup warning
"control-plane admin API is UNAUTHENTICATED" disappears.

3. Let the admin web UI sign in
-------------------------------

With ``admin_auth`` on, the UI needs its own sign-in flow — an Authorization
Code + PKCE login against your provider, with a public client:

.. code-block:: toml

   [admin_auth.login]
   client_id = "mcplake-admin-ui"
   authorization_endpoint = "https://sso.example.com/realms/main/protocol/openid-connect/auth"
   token_endpoint = "https://sso.example.com/realms/main/protocol/openid-connect/token"

Register the UI's URL (with a trailing slash) as a redirect URI on that
client. See :doc:`configure-keycloak` for a worked example.

4. Put TLS in front
-------------------

The gateway serves plain HTTP. If operators reach the control plane over a
network, terminate TLS in front of it — the tokens it receives are
administrator credentials. See :doc:`deploy-in-production`.

What is protected already
-------------------------

- Writes (``POST``, ``PUT``, ``PATCH``) must be ``Content-Type:
  application/json`` or are refused with ``415``, and the control plane sends
  no CORS headers — so a web page an operator visits cannot drive the API
  from their browser.
- The UI keeps its token in session storage and sends it as a header; there
  are no cookies to steal or replay.

See :doc:`/architecture/security` for the full model.
