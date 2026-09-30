Troubleshooting
===============

Organized by what you see. Every data-plane error has the body
``{"error": "<code>", "message": "..."}``; the code is the quickest pointer to
the cause. The full list is in :doc:`/reference/data-plane-api`.

A caller gets 401
-----------------

``missing_authorization`` / ``invalid_authorization``
   No ``Authorization`` header, or not ``Bearer <token>``. Some MCP clients
   drop headers configured for another transport — check the client's
   configuration for the endpoint you are using.

``unauthorized``
   The token failed validation. Decode it and compare with ``[oidc]``:

   .. code-block:: bash

      echo "$TOKEN" | cut -d. -f2 | tr '_-' '/+' | base64 -d 2>/dev/null | jq '{iss, aud, exp}'

   - ``iss`` must equal ``oidc.issuer`` exactly — watch for a trailing slash
     or ``http`` vs ``https``.
   - ``aud`` must contain ``oidc.audience``. Many providers set the audience
     only when a specific scope or audience mapper is applied.
   - ``exp`` must be in the future — tokens pasted into client configuration
     expire.
   - The signing key must be in the JWKS. A token signed with a key ID the
     gateway has not seen triggers a JWKS refresh (at most once a minute), so
     a rotation is picked up without a restart.

A caller gets 403
-----------------

``forbidden``
   The token is valid, but no enabled access policy both matches the claims
   and grants this ``(mcp, tool)``.

   - Compare the policy's ``match`` rules with the decoded token. All rules
     must match; patterns are case-sensitive, and unanchored patterns match
     substrings.
   - Check the grant names the *registration* name and the server's own tool
     name — not the ``<mcp>__<tool>`` name an MCP client shows.
   - Check the policy is ``enabled``.
   - The admin web UI's **Request path** view simulates a call for a given
     token payload.

``mcp_disabled``
   The MCP server is switched off. ``PATCH /admin/mcps/<name>`` with
   ``{"enabled": true}``.

An MCP client sees no tools, or too few
---------------------------------------

The catalogue contains only tools that are granted **and** whose server is
``active`` and enabled. Check, in order: the token (as for ``403``), the
server's status in ``GET /admin/mcps``, and its ``enabled`` flag. An empty
list with no error means the token is valid but nothing is granted to it.

A call fails with 404, 502, 503 or 504
--------------------------------------

``mcp_not_found`` / ``tool_not_found``
   The caller *is* authorized, but the registration name or tool does not
   exist. Check spelling against ``GET /admin/mcps``.

``mcp_unavailable`` (503)
   The server is registered but not connected. The health check is already
   retrying it; the log shows ``mcp is no longer reachable`` or
   ``mcp health check could not reconnect`` with the reason. Fix the server;
   the gateway reconnects by itself.

``upstream_error`` (502)
   The server was reached and the call failed. The detail is in the server's
   own logs. The gateway does not capture a ``stdio`` server's standard
   error, so have such a server log to a file (via its arguments or ``env``)
   when you need to debug it.

``filter_unenforceable`` (502)
   A filter policy applies to this call, but the tool returned non-JSON text
   that fields cannot be removed from. The gateway refuses rather than leak;
   remove the filter or restrict access to the tool instead. See
   :doc:`/concepts/response-filtering`.

``upstream_timeout`` (504)
   The server did not answer within 30 seconds.

A field is not being removed
----------------------------

- The filter's ``mcp`` and ``tool`` must match exactly — no wildcards.
- Its ``match`` rules must hold for the caller.
- Paths are relative to the tool's result, not the MCP envelope:
  ``$.salary``, not ``$.structuredContent.salary``.
- Look for ``filter policy matched but removed no fields`` in the log: the
  policy applied but its paths matched nothing — usually a renamed field or a
  list without ``[*]``.

A change to config.toml has no effect
-------------------------------------

For ``[[mcps]]`` and policies this is by design: they are seeded once, and the
database is in force afterwards. The log says
``config entry already seeded and DIFFERS from the stored record`` and names
the entry. Make the change through the admin API; see
:doc:`/concepts/configuration-and-state`.

For every other section, restart the gateway.

The gateway does not start
--------------------------

``mcplake: load config ...: config: parse ...``
   The file is not valid TOML.

``mcplake: load config ...: config: access_policies[N] ... match[M]: ...``
   A JSONPath expression or regular expression does not compile; the message
   names the entry.

``... listen tcp ...: bind: address already in use``
   Another process (often a previous gateway) holds the port.

An MCP server stays unreachable at startup
------------------------------------------

The gateway does not fail when a server cannot be reached; it records it as
``unreachable`` and keeps retrying. The log line
``mcp registration failed`` carries the reason. For ``stdio`` servers, run the
exact ``command`` and ``arguments`` by hand as the gateway's user — and
remember the subprocess only gets ``PATH``, ``HOME``, ``LANG``, ``LC_ALL``,
``TZ`` and ``TMPDIR`` from the gateway's environment, plus its own ``env``.

The admin UI keeps asking to sign in, or shows 403
--------------------------------------------------

- 403 after signing in: the account's token does not satisfy
  ``admin_auth.match``.
- Redirect errors at the provider: the UI's URL (with trailing slash) must be
  a registered redirect URI of ``admin_auth.login.client_id``.
- "Sign-in isn't configured": ``[admin_auth.login]`` is missing.
