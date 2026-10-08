Authentication
==============

mcplake does not issue identities. It trusts one OIDC provider — yours — and
verifies every request against it. Both planes use the same provider,
configured once in ``[oidc]``.

Tokens
------

A caller sends an access token from your provider as a bearer token:

.. code-block:: text

   Authorization: Bearer eyJhbGciOiJSUzI1NiIs...

The gateway accepts it when:

- its signature verifies against a key in the provider's JWKS
  (``oidc.jwks_url``, cached for ``oidc.jwks_cache_ttl``, default one hour);
- it has not expired;
- its ``iss`` equals ``oidc.issuer`` and its ``aud`` includes
  ``oidc.audience``.

Anything else is ``401``. After that the token's claims are all the gateway
knows about the caller, and all it needs: every decision is a
:doc:`claim rule <claim-rules>` over them.

How callers get tokens is between them and your provider. Agents running as a
service typically use the client-credentials grant; agents acting for a
person use a token obtained through that person's sign-in. Either way, make
sure the provider puts the claims your policies need into the **access token**
and sets its audience to what the gateway expects.

The data plane
--------------

Every data-plane request except ``/healthz`` requires a valid token. On the
MCP endpoints:

- the token is required to **open** a session;
- **authorization is re-evaluated on every request** — revoking a grant or
  disabling an MCP applies to the next ``tools/list`` or ``tools/call`` in an
  open session, without reconnecting;
- **identity is fixed when the session opens.** On SSE, clients do not resend
  the header on each message, so an SSE session can outlive its token's
  expiry until the stream drops. See the data-plane reference for the exact
  behaviour.

The control plane
-----------------

The admin API can start processes on the gateway host (by registering a
``stdio`` MCP), so treat access to it like shell access. Protect it with
``admin_auth``: claim rules that an administrator's token must satisfy, on top
of normal token validation.

.. code-block:: toml

   [[admin_auth.match]]
   path = "$.realm_access.roles[*]"
   pattern = "^mcplake-admin$"

.. warning::

   Without ``admin_auth.match`` rules the control plane is **unauthenticated**
   (the gateway warns at startup). Keep ``control_plane_addr`` on a loopback or
   otherwise private interface until it is configured, and even then prefer a
   trusted network over exposing it widely.

The admin web UI signs operators in with an Authorization Code + PKCE flow
against the same provider — configure
:ref:`[admin_auth.login] <reference-configuration-admin-ui-sign-in>`.

Transport security
------------------

The gateway serves plain HTTP on both listeners; it does not terminate TLS.
Run it behind a TLS-terminating proxy or load balancer, and keep the hop from
that proxy to the gateway on a trusted network.

Outbound, the gateway insists on ``https`` (or loopback ``http``) for the JWKS
URL, the admin login endpoints and network MCP servers.

Reference
---------

- :doc:`/architecture/security` — trust boundaries and known gaps.
- :ref:`reference-configuration-2-oidc-configuration` and the ``admin_auth``
  keys in :doc:`/reference/configuration`.
- :doc:`/architecture/decisions/0010-control-plane-admin-authentication`,
  :doc:`/architecture/decisions/0014-admin-ui-oidc-pkce-login`.
