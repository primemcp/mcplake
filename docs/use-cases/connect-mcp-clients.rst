Connect MCP Clients to the Gateway
==================================

The problem
-----------

Every developer's editor and every agent has its own list of MCP servers, each
configured separately, many holding their own credentials. Adding a server
means touching every client; revoking someone's access means finding every
copy.

The approach
------------

Configure clients with **one** MCP server — the gateway — and a token from
your identity provider. The gateway presents each client a single catalogue of
every tool its token is granted, named ``<mcp>__<tool>``. Adding a server or
changing someone's access is then done once, on the gateway.

.. list-table::
   :header-rows: 1
   :widths: 30 70

   * - Endpoint
     - Use
   * - ``https://<gateway>/v1/mcp``
     - MCP Streamable HTTP — use this unless the client lacks it.
   * - ``https://<gateway>/v1/sse``
     - MCP over SSE, for clients that only speak the older transport.

Both require ``Authorization: Bearer <token>``.

Tokens
------

The gateway does not implement MCP's OAuth discovery flow, so a client cannot
sign in by itself: it has to be **given** an access token to send as a header.
Obtain one from your identity provider — for a person, through their normal
sign-in (for example your provider's CLI); for a service, with the
client-credentials grant.

.. important::

   Access tokens expire, often within minutes to an hour. A token pasted into
   a configuration file stops working when it does. Prefer clients that can
   read the header from an environment variable or a command, and refresh the
   token outside the client.

Claude Code
-----------

.. code-block:: bash

   claude mcp add --transport http mcplake https://gateway.example.com/v1/mcp \
     --header "Authorization: Bearer $MCPLAKE_TOKEN"

The tools appear as ``mcp__mcplake__<mcp>__<tool>`` in Claude Code's own
naming.

VS Code
-------

In ``.vscode/mcp.json``, prompt for the token rather than storing it:

.. code-block:: json

   {
     "inputs": [
       { "type": "promptString", "id": "mcplake-token",
         "description": "mcplake access token", "password": true }
     ],
     "servers": {
       "mcplake": {
         "type": "http",
         "url": "https://gateway.example.com/v1/mcp",
         "headers": { "Authorization": "Bearer ${input:mcplake-token}" }
       }
     }
   }

Other clients
-------------

Most clients accept the same three things — transport, URL, headers — in a
JSON entry like this:

.. code-block:: javascript

   {
     "type": "http",              // or "sse" with the /v1/sse URL
     "url": "https://gateway.example.com/v1/mcp",
     "headers": { "Authorization": "Bearer <token>" }
   }

Your own agent
--------------

With the Python `FastMCP <https://gofastmcp.com>`_ client:

.. code-block:: python

   import asyncio, os
   from fastmcp import Client
   from fastmcp.client.transports import StreamableHttpTransport

   async def main():
       transport = StreamableHttpTransport(
           "https://gateway.example.com/v1/mcp",
           headers={"Authorization": f"Bearer {os.environ['MCPLAKE_TOKEN']}"},
       )
       async with Client(transport) as client:
           tools = await client.list_tools()
           print([t.name for t in tools])      # e.g. ['crm__search', 'tickets__get_ticket']
           result = await client.call_tool("tickets__get_ticket", {"id": "OPS-42"})
           print(result.structured_content)

   asyncio.run(main())

Agents that do not speak MCP can use the plain REST endpoint,
``POST /v1/call``, with the same token; see :doc:`/reference/data-plane-api`.

What to expect
--------------

- **The tool list is per caller.** Two people with the same client
  configuration see different tools if their tokens carry different claims.
  An empty list means the token is valid but nothing is granted to it.
- **Policy changes apply immediately**, even in an open session. Tools that
  appear or disappear are picked up when the client next lists tools.
- **Errors are MCP errors.** A call the caller is not allowed to make fails as
  a JSON-RPC error; a call that reached the server and failed (or timed out)
  returns a result with ``isError: true``.
- **Transport security.** The gateway serves plain HTTP; put it behind a TLS
  proxy so that ``https://`` URLs like the ones above reach it.

See also
--------

- :ref:`reference-data-plane-api-mcp-endpoints` — protocol
  details and session behaviour.
- :doc:`/concepts/authentication`
